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

func TestParseMemUsageKB(t *testing.T) {
	// ~63 GiB total, ~55.8 GiB available → ~9.5 GiB used.
	meminfo := "MemTotal:       65805020 kB\nMemFree:        40000000 kB\nMemAvailable:   55805020 kB\nBuffers:          100 kB\n"
	used, total := parseMemUsageKB(meminfo)
	if total != 65805020 {
		t.Errorf("total = %d, want 65805020", total)
	}
	if used != 10000000 {
		t.Errorf("used = %d, want 10000000 (total minus available)", used)
	}
	if u, tot := parseMemUsageKB("garbage"); u != 0 || tot != 0 {
		t.Errorf("parseMemUsageKB(garbage) = (%d, %d), want (0, 0)", u, tot)
	}
}

func TestParseDrives(t *testing.T) {
	// Two disks: one with plain partitions, one where root sits under
	// LUKS→LVM (so the mount point is two levels down), plus an unmounted disk.
	data := []byte(`{"blockdevices":[
	  {"name":"sda","type":"disk","size":2000398934016,"model":"Samsung SSD 870 EVO 2TB","children":[
	    {"name":"sda1","type":"part","size":499999834112,"mountpoint":"/mnt/Data","fstype":"ext4"}]},
	  {"name":"nvme0n1","type":"disk","size":2000398934016,"model":"Samsung SSD 990 PRO 2TB","children":[
	    {"name":"nvme0n1p1","type":"part","size":536870912,"mountpoint":"/boot/efi","fstype":"vfat"},
	    {"name":"nvme0n1p3","type":"part","size":1999000000000,"fstype":"crypto_LUKS","children":[
	      {"name":"crypt","type":"crypt","size":1999000000000,"fstype":"LVM2_member","children":[
	        {"name":"vgmint-root","type":"lvm","size":1999000000000,"mountpoint":"/","fstype":"ext4"}]}]}]},
	  {"name":"nvme1n1","type":"disk","size":2000398934016,"model":"Windows Disk","children":[
	    {"name":"nvme1n1p3","type":"part","size":1900000000000,"fstype":"ntfs"}]}]}`)

	drives := parseDrives(data)
	if len(drives) != 3 {
		t.Fatalf("got %d drives, want 3: %+v", len(drives), drives)
	}
	if !drives[1].Mounted || len(drives[1].Carries) != 2 ||
		drives[1].Carries[0] != "/" || drives[1].Carries[1] != "/boot/efi" {
		t.Errorf("LUKS/LVM drive should surface / and /boot/efi, got %v", drives[1].Carries)
	}
	if drives[2].Mounted {
		t.Error("a drive with no mounted partition must report Mounted=false")
	}
	if len(drives[2].Carries) != 1 || drives[2].Carries[0] != "ntfs" {
		t.Errorf("unmounted drive should fall back to its filesystems, got %v", drives[2].Carries)
	}
	// crypto_LUKS / LVM2_member name containers, not content — never surface them.
	for _, c := range drives[1].Carries {
		if c == "crypto_LUKS" || c == "LVM2_member" {
			t.Errorf("container type %q leaked into Carries", c)
		}
	}
	if drives[0].SizeKB != 2000398934016/1024 {
		t.Errorf("SizeKB = %d, want bytes/1024", drives[0].SizeKB)
	}
}

func TestParseDrivesIgnoresNonDisks(t *testing.T) {
	// A loop device (snap mounts) is not a drive.
	data := []byte(`{"blockdevices":[{"name":"loop0","type":"loop","size":1024,"mountpoint":"/snap/x"}]}`)
	if got := parseDrives(data); len(got) != 0 {
		t.Errorf("got %+v, want no drives", got)
	}
	if got := parseDrives([]byte("not json")); got != nil {
		t.Errorf("unparseable lsblk output should yield nil, got %+v", got)
	}
}
