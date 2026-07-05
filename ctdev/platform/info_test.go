package platform

import "testing"

func TestSnapToStandardSize(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{0, 4},
		{3, 4},
		{4, 4},
		{7, 8},
		{8, 8},
		{15, 16},
		{16, 16},
		{31, 32},
		{64, 64},
		{128, 128},
		{300, 300},
	}
	for _, tt := range tests {
		if got := snapToStandardSize(tt.input); got != tt.want {
			t.Errorf("snapToStandardSize(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseIPLink(t *testing.T) {
	output := `1: lo: <LOOPBACK,UP> mtu 65536 qdisc noqueue state UNKNOWN mode DEFAULT
2: enp0s31f6: <BROADCAST,MULTICAST,UP> mtu 1500 qdisc fq_codel state UP mode DEFAULT
3: wlp82s0: <BROADCAST,MULTICAST,UP> mtu 1500 qdisc noqueue state UP mode DORMANT
4: docker0: <NO-CARRIER,BROADCAST> mtu 1500 qdisc noqueue state DOWN mode DEFAULT
5: veth123abc@if6: <BROADCAST,MULTICAST,UP> mtu 1500 qdisc noqueue master docker0 state UP`

	adapters := parseIPLink(output)

	if len(adapters) != 2 {
		t.Fatalf("expected 2 adapters, got %d: %+v", len(adapters), adapters)
	}

	if adapters[0].Interface != "enp0s31f6" {
		t.Errorf("first adapter interface = %q, want enp0s31f6", adapters[0].Interface)
	}
	if adapters[0].Type != "ethernet" {
		t.Errorf("first adapter type = %q, want ethernet", adapters[0].Type)
	}

	if adapters[1].Interface != "wlp82s0" {
		t.Errorf("second adapter interface = %q, want wlp82s0", adapters[1].Interface)
	}
	if adapters[1].Type != "wifi" {
		t.Errorf("second adapter type = %q, want wifi", adapters[1].Type)
	}
}

func TestParseMacNetworkSetupEmpty(t *testing.T) {
	result := parseMacNetworkSetup("")
	if len(result) != 0 {
		t.Errorf("expected empty result for empty input, got %d", len(result))
	}
}

func TestParseMacNetworkSetupFiltersInactive(t *testing.T) {
	// All interfaces will be "inactive" since they don't exist
	input := `Hardware Port: Wi-Fi
Device: en_fake_wifi_99
VLAN Configurations

Hardware Port: Thunderbolt Ethernet
Device: en_fake_eth_99
VLAN Configurations
`
	result := parseMacNetworkSetup(input)
	// All should be filtered out since macIfaceActive returns false for fake interfaces
	if len(result) != 0 {
		t.Errorf("expected 0 adapters for fake interfaces, got %d", len(result))
	}
}

func TestParseCPUModel(t *testing.T) {
	x86 := "processor\t: 0\nvendor_id\t: AuthenticAMD\nmodel\t\t: 97\nmodel name\t: AMD Ryzen 9 9950X3D 16-Core Processor\n"
	if got := parseCPUModel(x86); got != "AMD Ryzen 9 9950X3D 16-Core Processor" {
		t.Errorf("x86 model = %q", got)
	}

	// arm64 Raspberry Pi: no per-core "model name", board "Model" at the end.
	pi := "processor\t: 0\nBogoMIPS\t: 108.00\nCPU implementer\t: 0x41\n\nHardware\t: BCM2712\nModel\t\t: Raspberry Pi 5 Model B Rev 1.0\n"
	if got := parseCPUModel(pi); got != "Raspberry Pi 5 Model B Rev 1.0" {
		t.Errorf("pi model = %q", got)
	}

	if got := parseCPUModel("processor\t: 0\n"); got != "" {
		t.Errorf("expected empty for cpuinfo without model lines, got %q", got)
	}
}
