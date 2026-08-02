package client

import (
	"AstraScheduleServerGo/model"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
)

// cache 缓存城市查询结果 (name, adm) -> LocationResp
var cache sync.Map

var errNoQWeatherCredential = errors.New("和风天气认证信息未配置")

// newRestyClient 创建 resty HTTP 客户端，测试时可替换为跳过 TLS 验证的版本
var newRestyClient = func() *resty.Client { return resty.New() }

type qweatherJWTHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

type qweatherJWTPayload struct {
	Sub string `json:"sub"`
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
}

func createQWeatherRequest(client *resty.Client, cfg model.APIKeyConfig) (*resty.Request, error) {
	req := client.R()

	if cfg.HasJWT() {
		token, err := generateQWeatherJWT(cfg.JWT)
		if err != nil {
			return nil, fmt.Errorf("生成 JWT 失败: %w", err)
		}
		req.SetHeader("Authorization", "Bearer "+token)
		return req, nil
	}

	if cfg.HasAPIKey() {
		req.SetHeader("X-QW-Api-Key", strings.TrimSpace(cfg.Weather))
		return req, nil
	}

	return nil, errNoQWeatherCredential
}

func parseEd25519PrivateKey(privateKeyPEM string) (ed25519.PrivateKey, error) {
	text := strings.TrimSpace(privateKeyPEM)
	if text == "" {
		return nil, fmt.Errorf("JWT 私钥不能为空")
	}

	text = strings.ReplaceAll(text, "\\n", "\n")

	var der []byte
	if strings.Contains(text, "-----BEGIN") {
		block, _ := pem.Decode([]byte(text))
		if block == nil {
			return nil, fmt.Errorf("JWT 私钥 PEM 解析失败")
		}
		der = block.Bytes
	} else {
		base64Text := strings.ReplaceAll(text, "\n", "")
		base64Text = strings.ReplaceAll(base64Text, "\r", "")
		base64Text = strings.ReplaceAll(base64Text, " ", "")

		var err error
		der, err = base64.StdEncoding.DecodeString(base64Text)
		if err != nil {
			der, err = base64.RawStdEncoding.DecodeString(base64Text)
			if err != nil {
				return nil, fmt.Errorf("JWT 私钥解析失败：请提供 PEM 或 PKCS8 DER 的 Base64 单行字符串")
			}
		}
	}

	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("JWT 私钥不是合法 PKCS8: %w", err)
	}

	privateKey, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("JWT 私钥不是 Ed25519")
	}

	return privateKey, nil
}

func generateQWeatherJWT(cfg model.JWTAuthConfig) (string, error) {
	privateKey, err := parseEd25519PrivateKey(cfg.PrivateKeyPEM)
	if err != nil {
		return "", err
	}

	now := time.Now().Unix()
	iat := now - 30
	expires := cfg.Expires
	if expires == 0 {
		expires = 900
	}
	exp := iat + expires

	headerBytes, err := json.Marshal(qweatherJWTHeader{
		Alg: "EdDSA",
		Kid: cfg.KID,
	})
	if err != nil {
		return "", fmt.Errorf("JWT Header 序列化失败: %w", err)
	}

	payloadBytes, err := json.Marshal(qweatherJWTPayload{
		Sub: cfg.ProjectID,
		Iat: iat,
		Exp: exp,
	})
	if err != nil {
		return "", fmt.Errorf("JWT Payload 序列化失败: %w", err)
	}

	headerEncoded := base64.RawURLEncoding.EncodeToString(headerBytes)
	payloadEncoded := base64.RawURLEncoding.EncodeToString(payloadBytes)
	signingInput := headerEncoded + "." + payloadEncoded

	signature := ed25519.Sign(privateKey, []byte(signingInput))
	signatureEncoded := base64.RawURLEncoding.EncodeToString(signature)

	return signingInput + "." + signatureEncoded, nil
}

// buildCityLookupURL 构建城市查询 URL
func buildCityLookupURL(name, adm, host string) string {
	if adm != "" {
		return fmt.Sprintf("https://%s/geo/v2/city/lookup?location=%s&adm=%s", host, url.QueryEscape(name), url.QueryEscape(adm))
	}
	return fmt.Sprintf("https://%s/geo/v2/city/lookup?location=%s", host, url.QueryEscape(name))
}

// parseLocationResult 解析城市查询结果
func parseLocationResult(result map[string]interface{}) (*model.LocationResp, error) {
	results, ok := result["location"].([]interface{})
	if !ok || len(results) == 0 {
		return nil, fmt.Errorf("未找到城市信息")
	}

	location, ok := results[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("城市信息格式错误")
	}

	id, idOk := location["id"].(string)
	latStr, latOk := location["lat"].(string)
	lonStr, lonOk := location["lon"].(string)
	name, nameOk := location["name"].(string)

	if !idOk || !latOk || !lonOk || !nameOk {
		return nil, fmt.Errorf("城市信息字段缺失")
	}

	lat, err1 := strconv.ParseFloat(latStr, 64)
	lon, err2 := strconv.ParseFloat(lonStr, 64)
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("经纬度解析失败")
	}

	return &model.LocationResp{ID: id, Lat: lat, Lon: lon, Name: name}, nil
}

// cityLookup 查询城市位置信息
func cityLookup(name, adm, host string, cfg model.APIKeyConfig) (*model.LocationResp, error) {
	if name == "" {
		return nil, fmt.Errorf("城市名不能为空")
	}

	cacheKey := fmt.Sprintf("%s_%s", name, adm)
	if cachedValue, ok := cache.Load(cacheKey); ok {
		cachedLoc := cachedValue.(*model.LocationResp)
		logrus.Infof("Cache hit: name = %s, adm = %s -> id: %s | lat: %f, lon: %f",
			name, adm, cachedLoc.ID, cachedLoc.Lat, cachedLoc.Lon)
		return cachedLoc, nil
	}

	url := buildCityLookupURL(name, adm, host)
	client := newRestyClient()
	req, err := createQWeatherRequest(client, cfg)
	if err != nil {
		return nil, err
	}
	resp, err := req.Get(url)
	if err != nil {
		return nil, fmt.Errorf("请求API失败: %w", err)
	}
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("API (%s) 请求失败，状态码: %d\n%s", url, resp.StatusCode(), resp.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("解析JSON响应失败: %w", err)
	}
	if code, ok := result["code"].(string); !ok || code != "200" {
		return nil, fmt.Errorf("API返回错误码: %v", result["code"])
	}

	locationResult, err := parseLocationResult(result)
	if err != nil {
		return nil, fmt.Errorf("未找到城市信息 (%s): %w", url, err)
	}

	cache.Store(cacheKey, locationResult)
	return locationResult, nil
}

// weatherLookup 查询指定位置的天气信息
func weatherLookup(location, host string, cfg model.APIKeyConfig) (*model.WeatherResp, error) {
	url := fmt.Sprintf("https://%s/v7/weather/now?location=%s", host, url.QueryEscape(location))

	client := newRestyClient()
	req, err := createQWeatherRequest(client, cfg)
	if err != nil {
		return nil, err
	}
	resp, err := req.Get(url)

	if err != nil {
		return nil, fmt.Errorf("请求天气API失败: %w", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("天气API (%s) 请求失败，状态码: %d", url, resp.StatusCode())
	}

	var result model.WeatherResp
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("解析天气JSON响应失败: %w", err)
	}

	if result.Now.Temp == "" {
		return nil, fmt.Errorf("未获取到天气信息")
	}

	return &result, nil
}

// weatherLookupByName 查询天气信息
func weatherLookupByName(name, adm, host string, cfg model.APIKeyConfig) (*model.WeatherResp, error) {
	// 首先通过城市名称获取城市ID
	locationInfo, err := cityLookup(name, adm, host, cfg)
	if err != nil {
		return nil, fmt.Errorf("获取城市信息失败: %w", err)
	}

	// 使用城市ID查询天气
	return weatherLookup(locationInfo.ID, host, cfg)
}

// weatherWarningLookup 查询指定位置的天气预警信息
func weatherWarningLookup(lat, lon, host string, cfg model.APIKeyConfig) (*model.WarningResp, error) {
	url := fmt.Sprintf("https://%s/weatheralert/v1/current/%s/%s", host, url.PathEscape(lat), url.PathEscape(lon))

	client := newRestyClient()
	req, err := createQWeatherRequest(client, cfg)
	if err != nil {
		return nil, err
	}
	resp, err := req.Get(url)

	if err != nil {
		return nil, fmt.Errorf("请求天气预警API失败: %w", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("天气预警API (%s) 请求失败，状态码: %d", url, resp.StatusCode())
	}

	var result model.WarningResp
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("解析预警JSON响应失败: %w", err)
	}

	return &result, nil
}

// weatherWarningLookupByName 查询天气预警信息
func weatherWarningLookupByName(name, adm, host string, cfg model.APIKeyConfig) (*model.WarningResp, error) {
	// 首先通过城市名称获取城市信息
	locationInfo, err := cityLookup(name, adm, host, cfg)
	if err != nil {
		return nil, fmt.Errorf("获取城市信息失败: %w", err)
	}

	// 使用城市经纬度查询天气预警
	lat := fmt.Sprintf("%.5f", locationInfo.Lat) // 保留5位小数
	lon := fmt.Sprintf("%.5f", locationInfo.Lon) // 保留5位小数
	return weatherWarningLookup(lat, lon, host, cfg)
}

func buildWeatherResponse(locationResp *model.LocationResp, warnResp *model.WarningResp, resp *model.WeatherResp) model.WeatherResponse {
	var warnParts []string
	var briefWarnParts []string
	for _, alert := range warnResp.Alerts {
		warnParts = append(warnParts, strings.ReplaceAll(alert.Description, "\n", ""))
		briefWarnParts = append(briefWarnParts, strings.ReplaceAll(alert.Headline, "\n", ""))
	}
	return model.WeatherResponse{
		Where:     locationResp.Name,
		Temp:      resp.Now.Temp,
		Weat:      resp.Now.Text,
		Wind:      resp.Now.WindDir,
		WindPower: resp.Now.WindScale,
		Warn:      strings.Join(warnParts, "；"),
		BriefWarn: strings.Join(briefWarnParts, "；"),
	}
}

func tryGetWeatherOnce(c *gin.Context, name, province, host string, apiCfg model.APIKeyConfig) bool {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("获取天气信息时发生 panic: %v", r)
		}
	}()

	locationResp, err := cityLookup(name, province, host, apiCfg)
	if err != nil {
		handleWeatherError(c, err)
		return true
	}

	resp, err := weatherLookupByName(name, province, host, apiCfg)
	if err != nil {
		logrus.Errorf("获取天气信息失败: %v", err)
		// 每次重试失败各计一次，语义为「天气上游请求失败次数」
		recordWeatherError(requestNamespace(c))
		return false
	}

	warnResp, _ := weatherWarningLookupByName(name, province, host, apiCfg)
	if warnResp == nil {
		warnResp = &model.WarningResp{Alerts: []model.Alert{}}
	}

	logrus.Infof(
		"获取 %s/%s（%s） 的天气信息，T: %s, W: %s, Warning: %s, Brief: %s",
		province, name, locationResp.Name, resp.Now.Temp, resp.Now.Text,
		resp.Now.WindDir, resp.Now.WindScale,
	)

	c.JSON(http.StatusOK, buildWeatherResponse(locationResp, warnResp, resp))
	return true
}

func handleWeatherError(c *gin.Context, err error) {
	if errors.Is(err, errNoQWeatherCredential) {
		logrus.Errorf("天气认证未配置: %v", err)
		c.JSON(http.StatusForbidden, gin.H{
			"error": "未配置天气认证信息：请配置 JWT（kid/project_id/private_key_pem）或 apikey.weather",
		})
		return
	}
	// 天气认证配置缺失不计数（属配置问题而非上游错误），避免管理端误报
	recordWeatherError(requestNamespace(c))
	logrus.Errorf("获取城市位置失败: %v", err)
	c.JSON(http.StatusNotFound, gin.H{
		"temp":       "404",
		"weat":       "不存在",
		"warning":    "",
		"brief_warn": "",
	})
}

func getWeatherOnce(c *gin.Context, name, province string) bool {
	apiCfg := model.Configs.APIKey
	host := apiCfg.APIHost
	return tryGetWeatherOnce(c, name, province, host, apiCfg)
}

func getWeather(c *gin.Context, name, province string) {
	logrus.Infof("将查询 %s 省 %s 市的天气信息", province, name)
	for i := 0; i < 5; i++ {
		if getWeatherOnce(c, name, province) {
			return
		}
		if c.Writer.Written() {
			return
		}
	}

	logrus.Errorf("获取 %s/%s 的天气信息失败，超过最大重试次数", province, name)
	c.JSON(http.StatusBadGateway, gin.H{
		"error": "获取天气信息失败，超过最大重试次数，可能是上游服务器异常，或是本服务器存在网络波动",
	})
}

// GetWeatherWithProvince 获取指定省份和城市的天气信息
func GetWeatherWithProvince(c *gin.Context) {
	name1 := c.Param("name1") // 省
	name2 := c.Param("name2") // 市
	if name2 == "" {
		getWeather(c, name1, "")
	} else {
		getWeather(c, name2, name1)
	}
}

// GetWeatherWithCity 获取指定省份和城市的天气信息
func GetWeatherWithCity(c *gin.Context) {
	name1 := c.Param("name1") // 市
	getWeather(c, name1, "")
}

// GetWeatherWithCFHeader 通过 Cloudflare 请求头获取天气信息
func GetWeatherWithCFHeader(c *gin.Context) {
	cfCity := c.GetHeader("CF-IPCity")
	cfRegion := c.GetHeader("CF-Region")

	if cfCity == "" {
		logrus.Errorf("无法通过请求头获取城市信息")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无法通过请求头获取城市信息，请确保请求经过 Cloudflare",
		})
		return
	}

	province := cfRegion
	if province == "" {
		province = ""
	}

	getWeather(c, cfCity, province)
}
