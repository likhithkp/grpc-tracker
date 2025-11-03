package grpctracker

// Import-only gRPC packet sniffer.
// Usage: in main app, add:
//   import _ "yourmodule/grpctracker"
// Then run the process with permission to capture packets (sudo or setcap).

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"regexp"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

const (
	snaplen    = 65535
	promisc    = false
	timeoutSec = 30
	grpcPort   = 4430 // default gRPC port to sniff
)

var methodRegex = regexp.MustCompile(`/[A-Za-z0-9_.]+/[A-Za-z0-9_]+`)

func init() {
	go func() {
		time.Sleep(1 * time.Second) // wait for app to boot
		startPcapSniffer()
	}()
}

func startPcapSniffer() {
	if os.Geteuid() != 0 {
		log.Printf("[grpc-tracker] ⚠️  not running as root — may need CAP_NET_RAW to capture packets. Try sudo or setcap.")
	}

	ifaces, err := pcap.FindAllDevs()
	if err != nil {
		log.Printf("[grpc-tracker] ❌ FindAllDevs: %v", err)
		return
	}

	// Prefer loopback interface
	var device string
	for _, i := range ifaces {
		for _, addr := range i.Addresses {
			if addr.IP.IsLoopback() {
				device = i.Name
				break
			}
		}
		if device != "" {
			break
		}
	}
	if device == "" {
		if len(ifaces) == 0 {
			log.Printf("[grpc-tracker] ❌ no devices found for pcap")
			return
		}
		device = ifaces[0].Name
	}

	log.Printf("[grpc-tracker] 🎧 capturing on %s for tcp port %d", device, grpcPort)

	handle, err := pcap.OpenLive(device, snaplen, promisc, pcap.BlockForever)
	if err != nil {
		log.Printf("[grpc-tracker] ❌ OpenLive: %v", err)
		return
	}
	defer handle.Close()

	bpf := fmt.Sprintf("tcp port %d", grpcPort)
	if err := handle.SetBPFFilter(bpf); err != nil {
		log.Printf("[grpc-tracker] ⚠️  failed to set BPF filter (%s): %v", bpf, err)
	}

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	seen := map[string]time.Time{}

	for pkt := range packetSource.Packets() {
		processPacket(pkt, seen)
	}
}

func processPacket(pkt gopacket.Packet, seen map[string]time.Time) {
	tcpLayer := pkt.Layer(layers.LayerTypeTCP)
	if tcpLayer == nil {
		return
	}
	tcp, _ := tcpLayer.(*layers.TCP)
	if tcp == nil || len(tcp.Payload) == 0 {
		return
	}
	payload := tcp.Payload

	// Check for /Service/Method patterns
	if bytes.Contains(payload, []byte("/")) {
		matches := methodRegex.FindAll(payload, -1)
		for _, m := range matches {
			method := string(m)
			if t, ok := seen[method]; ok && time.Since(t) < 5*time.Second {
				continue
			}
			seen[method] = time.Now()
			log.Printf("[grpc-tracker] 🚀 detected gRPC method: %s", method)
		}
	}

	// Heuristic: detect :path headers
	if bytes.Contains(payload, []byte(":path")) {
		idx := bytes.Index(payload, []byte(":path"))
		if idx >= 0 {
			sn := payload[idx:]
			if loc := bytes.Index(sn, []byte("/")); loc >= 0 {
				rest := sn[loc:]
				end := bytes.IndexAny(rest, "\r\n \x00")
				if end > 0 {
					cand := string(rest[:end])
					if methodRegex.MatchString(cand) {
						method := cand
						if t, ok := seen[method]; !ok || time.Since(t) >= 5*time.Second {
							seen[method] = time.Now()
							log.Printf("[grpc-tracker] 🔎 detected (path) gRPC method: %s", method)
						}
					}
				}
			}
		}
	}
}
