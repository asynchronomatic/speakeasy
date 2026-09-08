package autoip

import (
	"fmt"
	"net"

	"github.com/asynchronomatic/speakeasy/pkg/jsonrpc"
	"github.com/asynchronomatic/speakeasy/pkg/log"
)

var cloudflareRadarURL = "https://ipv4-check-perf.radar.cloudflare.com"
var outboundAddress = "8.8.8.8:80"

type CloudflareRadar struct {
	Colo      string `json:"colo"`
	Asn       int    `json:"asn"`
	Continent string `json:"continent"`
	Country   string `json:"country"`
	Region    string `json:"region"`
	City      string `json:"city"`
	Latitude  string `json:"latitude"`
	Longitude string `json:"longitude"`
	IPAddress string `json:"ip_address"`
	IPVersion string `json:"ip_version"`
}

type AddressDiscovery struct {
	Public   string
	Outbound string
}

func (ad *AddressDiscovery) IsPublic() bool {
	return ad.Outbound == ad.Public
}

func (ad *AddressDiscovery) IsNAT() bool {
	return ad.Outbound != ad.Public
}

func OutboundIP() (string, error) {
	conn, err := net.Dial("udp", outboundAddress)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	udpAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return "", fmt.Errorf("unexpected local addr type: %T", conn.LocalAddr())
	}
	return udpAddr.IP.String(), nil
}

func GetPublicAddress() (*AddressDiscovery, error) {
	client := jsonrpc.NewClient(cloudflareRadarURL, "")
	defer client.Close()

	info := &CloudflareRadar{}
	err := client.Get("/api/info", &info)
	if err != nil {
		return nil, err
	}

	outbound, err := OutboundIP()
	if err != nil {
		return nil, err
	}

	dc := &AddressDiscovery{
		Public:   info.IPAddress,
		Outbound: outbound,
	}

	log.WithName("autoip").Infof("AutoIP:  Public:%s  Outbound:%s  [IsPublic:%v]", dc.Public, dc.Outbound, dc.IsPublic())
	return dc, nil
}
