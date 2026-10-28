package mediafs

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/syudead/vv/internal/domain"
)

// fakeSysfs lays out sysfs as Linux does: /sys/dev/block/<maj>:<min> links to the device's directory, and a
// partition's directory sits inside its whole disk's.
func fakeSysfs(t *testing.T) string {
	t.Helper()
	sys := t.TempDir()
	write := func(path, content string) {
		t.Helper()
		full := filepath.Join(sys, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	link := func(device, target string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(sys, "dev", "block"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join("..", "..", target), filepath.Join(sys, "dev", "block", device)); err != nil {
			t.Fatal(err)
		}
	}
	write("devices/sda/dev", "8:0\n")
	write("devices/sda/queue/rotational", "1\n")
	write("devices/sda/sda1/dev", "8:1\n")
	write("devices/sda/sda1/partition", "1\n")
	link("8:1", "devices/sda/sda1")
	write("devices/nvme0n1/dev", "259:0\n")
	write("devices/nvme0n1/queue/rotational", "0\n")
	link("259:0", "devices/nvme0n1")
	return sys
}

func TestDiskOfDeviceReadsTheWholeDiskFromSysfs(t *testing.T) {
	sys := fakeSysfs(t)
	for _, tc := range []struct {
		name  string
		major uint32
		minor uint32
		want  domain.Disk
	}{
		{name: "partition of a rotational disk", major: 8, minor: 1,
			want: domain.Disk{Key: "block:8:0", Type: domain.DiskRotational}},
		{name: "whole solid-state disk", major: 259, minor: 0,
			want: domain.Disk{Key: "block:259:0", Type: domain.DiskSolidState}},
		// Btrfs and network file systems report an anonymous device sysfs does not list.
		{name: "device without a block entry", major: 0, minor: 45, want: domain.Disk{Key: "dev:0:45"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := diskOfDevice(sys, unix.Mkdev(tc.major, tc.minor)); got != tc.want {
				t.Fatalf("disk = %+v, want %+v", got, tc.want)
			}
		})
	}
}
