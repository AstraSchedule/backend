package client

import (
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

func resetCollectorForTest() {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	collector.day = ""
	collector.weatherErrors = map[string]int{}
	collector.wsDisconnects = map[string]map[string]int{}
}

func TestStatCollector_RecordAndSnapshot(t *testing.T) {
	resetCollectorForTest()

	recordWeatherError("ns-a")
	recordWeatherError("ns-a")
	recordWeatherError("ns-b")
	recordWSDisconnect("ns-a", "39/2023/1")
	recordWSDisconnect("ns-a", "39/2023/1")
	recordWSDisconnect("ns-a", "39/2023/2")
	recordWSDisconnect("ns-b", "7/2026/1")

	we, dis := statSnapshot("ns-a")
	assert.Equal(t, 2, we)
	assert.Equal(t, map[string]int{"39/2023/1": 2, "39/2023/2": 1}, dis)

	weB, disB := statSnapshot("ns-b")
	assert.Equal(t, 1, weB)
	assert.Equal(t, map[string]int{"7/2026/1": 1}, disB)

	// 空 namespace 与 "default" 读写一致（release 模式下 GetNamespace 可能返回空串）
	recordWeatherError("")
	recordWSDisconnect("", "39/2023/1")
	weD, disD := statSnapshot("default")
	assert.Equal(t, 1, weD)
	assert.Equal(t, map[string]int{"39/2023/1": 1}, disD)
	weE, _ := statSnapshot("")
	assert.Equal(t, 1, weE)
}

func TestStatCollector_RejectsOversizedClassKey(t *testing.T) {
	resetCollectorForTest()

	// 超长 classKey 直接拒绝，防伪造路径参数打爆内存
	recordWSDisconnect("ns-a", strings.Repeat("x", maxClassKeyLen+1))
	_, dis := statSnapshot("ns-a")
	assert.Equal(t, map[string]int{}, dis)

	// 超长 namespace 直接拒绝（天气错误与断连记录均需防护）
	recordWeatherError(strings.Repeat("y", maxNamespaceLen+1))
	we, _ := statSnapshot("default")
	assert.Equal(t, 0, we)
	recordWSDisconnect(strings.Repeat("z", maxNamespaceLen+1), "39/2023/1")
	_, dis2 := statSnapshot("default")
	assert.Equal(t, map[string]int{}, dis2)

	// 正常 key 不受影响
	recordWSDisconnect("ns-a", "39/2023/1")
	_, dis3 := statSnapshot("ns-a")
	assert.Equal(t, map[string]int{"39/2023/1": 1}, dis3)
}

func TestStatCollector_Rollover(t *testing.T) {
	resetCollectorForTest()

	recordWeatherError("ns-a")

	// 模拟跨天：把采集器日期拨回昨天
	collector.mu.Lock()
	collector.day = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	collector.mu.Unlock()

	we, dis := statSnapshot("ns-a")
	assert.Equal(t, 0, we)
	assert.Equal(t, map[string]int{}, dis)

	recordWeatherError("ns-a")
	we2, _ := statSnapshot("ns-a")
	assert.Equal(t, 1, we2)
}

func TestHubOnlineClasses(t *testing.T) {
	c1 := &websocket.Conn{}
	c2 := &websocket.Conn{}
	c3 := &websocket.Conn{}
	c4 := &websocket.Conn{}
	clientWsHub.add(wsScope{School: "39", Grade: "2023"}, "1", c1)
	clientWsHub.add(wsScope{School: "39", Grade: "2023"}, "1", c2) // 同班两个连接
	clientWsHub.add(wsScope{School: "39", Grade: "2023"}, "2", c3)
	clientWsHub.add(wsScope{School: "7", Grade: "2026"}, "1", c4)
	defer func() {
		clientWsHub.remove(wsScope{School: "39", Grade: "2023"}, c1)
		clientWsHub.remove(wsScope{School: "39", Grade: "2023"}, c2)
		clientWsHub.remove(wsScope{School: "39", Grade: "2023"}, c3)
		clientWsHub.remove(wsScope{School: "7", Grade: "2026"}, c4)
	}()

	classes := clientWsHub.onlineClasses("default")
	assert.Equal(t, map[string]bool{
		"39/2023/1": true,
		"39/2023/2": true,
		"7/2026/1":  true,
	}, classes)

	// main 分支 wsScope 无 namespace 概念，onlineClasses 忽略 namespace 参数，返回全部在线班级
	assert.Equal(t, classes, clientWsHub.onlineClasses("other-ns"))
}
