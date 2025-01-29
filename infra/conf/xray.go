package conf

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/xtls/xray-core/app/dispatcher"
	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/app/stats"
	"github.com/xtls/xray-core/common/errors"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/serial"
	core "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/transport/internet"
)

var (
	inboundConfigLoader = NewJSONConfigLoader(ConfigCreatorCache{
		"dokodemo-door": func() interface{} { return new(DokodemoConfig) },
		"http":          func() interface{} { return new(HTTPServerConfig) },
		"shadowsocks":   func() interface{} { return new(ShadowsocksServerConfig) },
		"mixed":         func() interface{} { return new(SocksServerConfig) },
		"socks":         func() interface{} { return new(SocksServerConfig) },
		"vless":         func() interface{} { return new(VLessInboundConfig) },
		"vmess":         func() interface{} { return new(VMessInboundConfig) },
		"trojan":        func() interface{} { return new(TrojanServerConfig) },
		"wireguard":     func() interface{} { return &WireGuardConfig{IsClient: false} },
	}, "protocol", "settings")

	outboundConfigLoader = NewJSONConfigLoader(ConfigCreatorCache{
		"blackhole":   func() interface{} { return new(BlackholeConfig) },
		"loopback":    func() interface{} { return new(LoopbackConfig) },
		"freedom":     func() interface{} { return new(FreedomConfig) },
		"http":        func() interface{} { return new(HTTPClientConfig) },
		"shadowsocks": func() interface{} { return new(ShadowsocksClientConfig) },
		"socks":       func() interface{} { return new(SocksClientConfig) },
		"vless":       func() interface{} { return new(VLessOutboundConfig) },
		"vmess":       func() interface{} { return new(VMessOutboundConfig) },
		"trojan":      func() interface{} { return new(TrojanClientConfig) },
		"dns":         func() interface{} { return new(DNSOutboundConfig) },
		"wireguard":   func() interface{} { return &WireGuardConfig{IsClient: true} },
	}, "protocol", "settings")

	ctllog = log.New(os.Stderr, "xctl> ", 0)
)

type SniffingConfig struct {
	Enabled         bool        `json:"enabled"`
	DestOverride    *StringList `json:"destOverride"`
	DomainsExcluded *StringList `json:"domainsExcluded"`
	MetadataOnly    bool        `json:"metadataOnly"`
	RouteOnly       bool        `json:"routeOnly"`
}

// Build implements Buildable.
func (c *SniffingConfig) Build() (*proxyman.SniffingConfig, error) {
	var p []string
	if c.DestOverride != nil {
		for _, protocol := range *c.DestOverride {
			switch strings.ToLower(protocol) {
			case "http":
				p = append(p, "http")
			case "tls", "https", "ssl":
				p = append(p, "tls")
			case "quic":
				p = append(p, "quic")
			case "fakedns":
				p = append(p, "fakedns")
			case "fakedns+others":
				p = append(p, "fakedns+others")
			default:
				return nil, errors.New("unknown protocol: ", protocol)
			}
		}
	}

	var d []string
	if c.DomainsExcluded != nil {
		for _, domain := range *c.DomainsExcluded {
			d = append(d, strings.ToLower(domain))
		}
	}

	return &proxyman.SniffingConfig{
		Enabled:             c.Enabled,
		DestinationOverride: p,
		DomainsExcluded:     d,
		MetadataOnly:        c.MetadataOnly,
		RouteOnly:           c.RouteOnly,
	}, nil
}

type MuxConfig struct {
	Enabled         bool   `json:"enabled"`
	Concurrency     int16  `json:"concurrency"`
	XudpConcurrency int16  `json:"xudpConcurrency"`
	XudpProxyUDP443 string `json:"xudpProxyUDP443"`
}

// Build creates MultiplexingConfig, Concurrency < 0 completely disables mux.
func (m *MuxConfig) Build() (*proxyman.MultiplexingConfig, error) {
	switch m.XudpProxyUDP443 {
	case "":
		m.XudpProxyUDP443 = "reject"
	case "reject", "allow", "skip":
	default:
		return nil, errors.New(`unknown "xudpProxyUDP443": `, m.XudpProxyUDP443)
	}
	return &proxyman.MultiplexingConfig{
		Enabled:         m.Enabled,
		Concurrency:     int32(m.Concurrency),
		XudpConcurrency: int32(m.XudpConcurrency),
		XudpProxyUDP443: m.XudpProxyUDP443,
	}, nil
}

type InboundDetourAllocationConfig struct {
	Strategy    string  `json:"strategy"`
	Concurrency *uint32 `json:"concurrency"`
	RefreshMin  *uint32 `json:"refresh"`
}

// Build implements Buildable.
func (c *InboundDetourAllocationConfig) Build() (*proxyman.AllocationStrategy, error) {
	config := new(proxyman.AllocationStrategy)
	switch strings.ToLower(c.Strategy) {
	case "always":
		config.Type = proxyman.AllocationStrategy_Always
	case "random":
		config.Type = proxyman.AllocationStrategy_Random
	case "external":
		config.Type = proxyman.AllocationStrategy_External
	default:
		return nil, errors.New("unknown allocation strategy: ", c.Strategy)
	}
	if c.Concurrency != nil {
		config.Concurrency = &proxyman.AllocationStrategy_AllocationStrategyConcurrency{
			Value: *c.Concurrency,
		}
	}

	if c.RefreshMin != nil {
		config.Refresh = &proxyman.AllocationStrategy_AllocationStrategyRefresh{
			Value: *c.RefreshMin,
		}
	}

	return config, nil
}

type InboundDetourConfig struct {
	Protocol       string                         `json:"protocol"`
	PortList       *PortList                      `json:"port"`
	ListenOn       *Address                       `json:"listen"`
	Settings       *json.RawMessage               `json:"settings"`
	Tag            string                         `json:"tag"`
	Allocation     *InboundDetourAllocationConfig `json:"allocate"`
	StreamSetting  *StreamConfig                  `json:"streamSettings"`
	SniffingConfig *SniffingConfig                `json:"sniffing"`
}

// Build implements Buildable.
func (c *InboundDetourConfig) Build() (*core.InboundHandlerConfig, error) {
	receiverSettings := &proxyman.ReceiverConfig{}

	if c.ListenOn == nil {
		// Listen on anyip, must set PortList
		if c.PortList == nil {
			return nil, errors.New("Listen on AnyIP but no Port(s) set in InboundDetour.")
		}
		receiverSettings.PortList = c.PortList.Build()
	} else {
		// Listen on specific IP or Unix Domain Socket
		receiverSettings.Listen = c.ListenOn.Build()
		listenDS := c.ListenOn.Family().IsDomain() && (filepath.IsAbs(c.ListenOn.Domain()) || c.ListenOn.Domain()[0] == '@')
		listenIP := c.ListenOn.Family().IsIP() || (c.ListenOn.Family().IsDomain() && c.ListenOn.Domain() == "localhost")
		if listenIP {
			// Listen on specific IP, must set PortList
			if c.PortList == nil {
				return nil, errors.New("Listen on specific ip without port in InboundDetour.")
			}
			// Listen on IP:Port
			receiverSettings.PortList = c.PortList.Build()
		} else if listenDS {
			if c.PortList != nil {
				// Listen on Unix Domain Socket, PortList should be nil
				receiverSettings.PortList = nil
			}
		} else {
			return nil, errors.New("unable to listen on domain address: ", c.ListenOn.Domain())
		}
	}

	if c.Allocation != nil {
		concurrency := -1
		if c.Allocation.Concurrency != nil && c.Allocation.Strategy == "random" {
			concurrency = int(*c.Allocation.Concurrency)
		}
		portRange := 0

		for _, pr := range c.PortList.Range {
			portRange += int(pr.To - pr.From + 1)
		}
		if concurrency >= 0 && concurrency >= portRange {
			var ports strings.Builder
			for _, pr := range c.PortList.Range {
				fmt.Fprintf(&ports, "%d-%d ", pr.From, pr.To)
			}
			return nil, errors.New("not enough ports. concurrency = ", concurrency, " ports: ", ports.String())
		}

		as, err := c.Allocation.Build()
		if err != nil {
			return nil, err
		}
		receiverSettings.AllocationStrategy = as
	}
	if c.StreamSetting != nil {
		ss, err := c.StreamSetting.Build()
		if err != nil {
			return nil, err
		}
		receiverSettings.StreamSettings = ss
	}
	if c.SniffingConfig != nil {
		s, err := c.SniffingConfig.Build()
		if err != nil {
			return nil, errors.New("failed to build sniffing config").Base(err)
		}
		receiverSettings.SniffingSettings = s
	}

	settings := []byte("{}")
	if c.Settings != nil {
		settings = ([]byte)(*c.Settings)
	}
	rawConfig, err := inboundConfigLoader.LoadWithID(settings, c.Protocol)
	if err != nil {
		return nil, errors.New("failed to load inbound detour config.").Base(err)
	}
	if dokodemoConfig, ok := rawConfig.(*DokodemoConfig); ok {
		receiverSettings.ReceiveOriginalDestination = dokodemoConfig.Redirect
	}
	ts, err := rawConfig.(Buildable).Build()
	if err != nil {
		return nil, err
	}

	return &core.InboundHandlerConfig{
		Tag:              c.Tag,
		ReceiverSettings: serial.ToTypedMessage(receiverSettings),
		ProxySettings:    serial.ToTypedMessage(ts),
	}, nil
}

type OutboundDetourConfig struct {
	Protocol      string           `json:"protocol"`
	SendThrough   *string          `json:"sendThrough"`
	Tag           string           `json:"tag"`
	Settings      *json.RawMessage `json:"settings"`
	StreamSetting *StreamConfig    `json:"streamSettings"`
	ProxySettings *ProxyConfig     `json:"proxySettings"`
	MuxSettings   *MuxConfig       `json:"mux"`
}

func (c *OutboundDetourConfig) checkChainProxyConfig() error {
	if c.StreamSetting == nil || c.ProxySettings == nil || c.StreamSetting.SocketSettings == nil {
		return nil
	}
	if len(c.ProxySettings.Tag) > 0 && len(c.StreamSetting.SocketSettings.DialerProxy) > 0 {
		return errors.New("proxySettings.tag is conflicted with sockopt.dialerProxy").AtWarning()
	}
	return nil
}

// Build implements Buildable.
func (c *OutboundDetourConfig) Build() (*core.OutboundHandlerConfig, error) {
	senderSettings := &proxyman.SenderConfig{}
	if err := c.checkChainProxyConfig(); err != nil {
		return nil, err
	}

	if c.SendThrough != nil {
		address := ParseSendThough(c.SendThrough)
		//Check if CIDR exists
		if strings.Contains(*c.SendThrough, "/") {
			senderSettings.ViaCidr = strings.Split(*c.SendThrough, "/")[1]
		} else {
			if address.Family().IsDomain() {
				return nil, errors.New("unable to send through: " + address.String())
			}
		}
		senderSettings.Via = address.Build()
	}

	if c.StreamSetting != nil {
		ss, err := c.StreamSetting.Build()
		if err != nil {
			return nil, err
		}
		senderSettings.StreamSettings = ss
	}

	if c.ProxySettings != nil {
		ps, err := c.ProxySettings.Build()
		if err != nil {
			return nil, errors.New("invalid outbound detour proxy settings.").Base(err)
		}
		if ps.TransportLayerProxy {
			if senderSettings.StreamSettings != nil {
				if senderSettings.StreamSettings.SocketSettings != nil {
					senderSettings.StreamSettings.SocketSettings.DialerProxy = ps.Tag
				} else {
					senderSettings.StreamSettings.SocketSettings = &internet.SocketConfig{DialerProxy: ps.Tag}
				}
			} else {
				senderSettings.StreamSettings = &internet.StreamConfig{SocketSettings: &internet.SocketConfig{DialerProxy: ps.Tag}}
			}
			ps = nil
		}
		senderSettings.ProxySettings = ps
	}

	if c.MuxSettings != nil {
		ms, err := c.MuxSettings.Build()
		if err != nil {
			return nil, errors.New("failed to build Mux config.").Base(err)
		}
		senderSettings.MultiplexSettings = ms
	}

	settings := []byte("{}")
	if c.Settings != nil {
		settings = ([]byte)(*c.Settings)
	}
	rawConfig, err := outboundConfigLoader.LoadWithID(settings, c.Protocol)
	if err != nil {
		return nil, errors.New("failed to parse to outbound detour config.").Base(err)
	}
	ts, err := rawConfig.(Buildable).Build()
	if err != nil {
		return nil, err
	}

	return &core.OutboundHandlerConfig{
		SenderSettings: serial.ToTypedMessage(senderSettings),
		Tag:            c.Tag,
		ProxySettings:  serial.ToTypedMessage(ts),
	}, nil
}

type StatsConfig struct{}

// Build implements Buildable.
func (c *StatsConfig) Build() (*stats.Config, error) {
	return &stats.Config{}, nil
}

type Config struct {
	// Deprecated: Global transport config is no longer used
	// left for returning error
	Transport map[string]json.RawMessage `json:"transport"`

	LogConfig        *LogConfig              `json:"log"`
	RouterConfig     *RouterConfig           `json:"routing"`
	DNSConfig        *DNSConfig              `json:"dns"`
	InboundConfigs   []InboundDetourConfig   `json:"inbounds"`
	OutboundConfigs  []OutboundDetourConfig  `json:"outbounds"`
	Policy           *PolicyConfig           `json:"policy"`
	API              *APIConfig              `json:"api"`
	Metrics          *MetricsConfig          `json:"metrics"`
	Stats            *StatsConfig            `json:"stats"`
	Reverse          *ReverseConfig          `json:"reverse"`
	FakeDNS          *FakeDNSConfig          `json:"fakeDns"`
	Observatory      *ObservatoryConfig      `json:"observatory"`
	BurstObservatory *BurstObservatoryConfig `json:"burstObservatory"`
}

func (c *Config) findInboundTag(tag string) int {
	found := -1
	for idx, ib := range c.InboundConfigs {
		if ib.Tag == tag {
			found = idx
			break
		}
	}
	return found
}

func (c *Config) findOutboundTag(tag string) int {
	found := -1
	for idx, ob := range c.OutboundConfigs {
		if ob.Tag == tag {
			found = idx
			break
		}
	}
	return found
}

// Override method accepts another Config overrides the current attribute
func (c *Config) Override(o *Config, fn string) {
	// only process the non-deprecated members

	// 以conf.Config的字段为单位，当传入的Config结构体的字段不为空，就用新的字段的值覆盖原来Config结构体的相同字段
	// InboundConfigs和OutboundConfigs字段除外（这两个字段是切片，需要比较切片的元素）
	if o.LogConfig != nil {
		c.LogConfig = o.LogConfig
	}
	if o.RouterConfig != nil {
		c.RouterConfig = o.RouterConfig
	}
	if o.DNSConfig != nil {
		c.DNSConfig = o.DNSConfig
	}
	if o.Transport != nil {
		c.Transport = o.Transport
	}
	if o.Policy != nil {
		c.Policy = o.Policy
	}
	if o.API != nil {
		c.API = o.API
	}
	if o.Metrics != nil {
		c.Metrics = o.Metrics
	}
	if o.Stats != nil {
		c.Stats = o.Stats
	}
	if o.Reverse != nil {
		c.Reverse = o.Reverse
	}

	if o.FakeDNS != nil {
		c.FakeDNS = o.FakeDNS
	}

	if o.Observatory != nil {
		c.Observatory = o.Observatory
	}

	if o.BurstObservatory != nil {
		c.BurstObservatory = o.BurstObservatory
	}

	// update the Inbound in slice if the only one in override config has same tag
	if len(o.InboundConfigs) > 0 {
		// 遍历传入的InboundConfigs的元素
		for i := range o.InboundConfigs {
			// 在原来的InboundConfigs中查找tag相同的元素，存在的话返回索引，不存在返回-1
			if idx := c.findInboundTag(o.InboundConfigs[i].Tag); idx > -1 {
				// 存在的话，用传入的tag元素覆盖idx位置的元素（即覆盖tag相同的元素）
				c.InboundConfigs[idx] = o.InboundConfigs[i]
				errors.LogInfo(context.Background(), "[", fn, "] updated inbound with tag: ", o.InboundConfigs[i].Tag)

			} else {
				// 不存在的话，在原来的InboundConfigs中追加这个tag项
				c.InboundConfigs = append(c.InboundConfigs, o.InboundConfigs[i])
				errors.LogInfo(context.Background(), "[", fn, "] appended inbound with tag: ", o.InboundConfigs[i].Tag)
			}

		}
	}

	// update the Outbound in slice if the only one in override config has same tag
	// 在 https://xtls.github.io/config/features/multiple.html 找到关于多文件启动的描述
	// 查找原有 tag 相同的元素进行覆盖；若无法找到：
	// 对于 inbounds，添加至最后（inbounds 内元素顺序无关）
	// 对于 outbounds，添加至最前（outbounds 默认首选出口）；但如果文件名含有 tail（大小写均可），添加至最后。
	if len(o.OutboundConfigs) > 0 {
		outboundPrepends := []OutboundDetourConfig{}
		for i := range o.OutboundConfigs {
			if idx := c.findOutboundTag(o.OutboundConfigs[i].Tag); idx > -1 {
				// tag相同元素存在的情况，和inbounds一样的逻辑
				c.OutboundConfigs[idx] = o.OutboundConfigs[i]
				errors.LogInfo(context.Background(), "[", fn, "] updated outbound with tag: ", o.OutboundConfigs[i].Tag)
			} else {
				if strings.Contains(strings.ToLower(fn), "tail") {
					// fn是文件名（file name？），如果文件名包含tail，表示元素添加到最后
					c.OutboundConfigs = append(c.OutboundConfigs, o.OutboundConfigs[i])
					errors.LogInfo(context.Background(), "[", fn, "] appended outbound with tag: ", o.OutboundConfigs[i].Tag)
				} else {
					// 否则添加到最前
					outboundPrepends = append(outboundPrepends, o.OutboundConfigs[i])
					errors.LogInfo(context.Background(), "[", fn, "] prepend outbound with tag: ", o.OutboundConfigs[i].Tag)
				}
			}
		}
		// 这里为什么要又判断一次文件名是否不包含tail？
		// 因为只有一个文件，如果文件名包含tail，那outboundPrepends一定是空
		// 反之，如果文件名不包含tail，那outboundPrepends一定不为空
		// 直接判断len(outboundPrepends)不行吗
		if !strings.Contains(strings.ToLower(fn), "tail") && len(outboundPrepends) > 0 {
			c.OutboundConfigs = append(outboundPrepends, c.OutboundConfigs...)
		}
	}
}

// Build implements Buildable.
func (c *Config) Build() (*core.Config, error) {
	if err := PostProcessConfigureFile(c); err != nil {
		return nil, err
	}

	config := &core.Config{
		App: []*serial.TypedMessage{
			serial.ToTypedMessage(&dispatcher.Config{}),
			serial.ToTypedMessage(&proxyman.InboundConfig{}),
			serial.ToTypedMessage(&proxyman.OutboundConfig{}),
		},
	}

	if c.API != nil {
		apiConf, err := c.API.Build()
		if err != nil {
			return nil, err
		}
		config.App = append(config.App, serial.ToTypedMessage(apiConf))
	}
	if c.Metrics != nil {
		metricsConf, err := c.Metrics.Build()
		if err != nil {
			return nil, err
		}
		config.App = append(config.App, serial.ToTypedMessage(metricsConf))
	}
	if c.Stats != nil {
		statsConf, err := c.Stats.Build()
		if err != nil {
			return nil, err
		}
		config.App = append(config.App, serial.ToTypedMessage(statsConf))
	}

	var logConfMsg *serial.TypedMessage
	if c.LogConfig != nil {
		logConfMsg = serial.ToTypedMessage(c.LogConfig.Build())
	} else {
		logConfMsg = serial.ToTypedMessage(DefaultLogConfig())
	}
	// let logger module be the first App to start,
	// so that other modules could print log during initiating
	config.App = append([]*serial.TypedMessage{logConfMsg}, config.App...)

	if c.RouterConfig != nil {
		routerConfig, err := c.RouterConfig.Build()
		if err != nil {
			return nil, err
		}
		config.App = append(config.App, serial.ToTypedMessage(routerConfig))
	}

	if c.DNSConfig != nil {
		dnsApp, err := c.DNSConfig.Build()
		if err != nil {
			return nil, errors.New("failed to parse DNS config").Base(err)
		}
		config.App = append(config.App, serial.ToTypedMessage(dnsApp))
	}

	if c.Policy != nil {
		pc, err := c.Policy.Build()
		if err != nil {
			return nil, err
		}
		config.App = append(config.App, serial.ToTypedMessage(pc))
	}

	if c.Reverse != nil {
		r, err := c.Reverse.Build()
		if err != nil {
			return nil, err
		}
		config.App = append(config.App, serial.ToTypedMessage(r))
	}

	if c.FakeDNS != nil {
		r, err := c.FakeDNS.Build()
		if err != nil {
			return nil, err
		}
		config.App = append([]*serial.TypedMessage{serial.ToTypedMessage(r)}, config.App...)
	}

	if c.Observatory != nil {
		r, err := c.Observatory.Build()
		if err != nil {
			return nil, err
		}
		config.App = append(config.App, serial.ToTypedMessage(r))
	}

	if c.BurstObservatory != nil {
		r, err := c.BurstObservatory.Build()
		if err != nil {
			return nil, err
		}
		config.App = append(config.App, serial.ToTypedMessage(r))
	}

	var inbounds []InboundDetourConfig

	if len(c.InboundConfigs) > 0 {
		inbounds = append(inbounds, c.InboundConfigs...)
	}

	if len(c.Transport) > 0 {
		return nil, errors.PrintRemovedFeatureError("Global transport config", "streamSettings in inbounds and outbounds")
	}

	for _, rawInboundConfig := range inbounds {
		ic, err := rawInboundConfig.Build()
		if err != nil {
			return nil, err
		}
		config.Inbound = append(config.Inbound, ic)
	}

	var outbounds []OutboundDetourConfig

	if len(c.OutboundConfigs) > 0 {
		outbounds = append(outbounds, c.OutboundConfigs...)
	}

	for _, rawOutboundConfig := range outbounds {
		oc, err := rawOutboundConfig.Build()
		if err != nil {
			return nil, err
		}
		config.Outbound = append(config.Outbound, oc)
	}

	return config, nil
}

// Convert string to Address.
func ParseSendThough(Addr *string) *Address {
	var addr Address
	addr.Address = net.ParseAddress(strings.Split(*Addr, "/")[0])
	return &addr
}
