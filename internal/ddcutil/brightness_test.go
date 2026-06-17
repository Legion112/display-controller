package ddcutil

import (
	"context"
	"testing"
)

func TestParseBrightnessBrief(t *testing.T) {
	b, err := parseBrightness("VCP 10 C 45 100\n")
	if err != nil {
		t.Fatal(err)
	}
	if b.Current != 45 || b.Max != 100 {
		t.Fatalf("got %+v, want current=45 max=100", b)
	}
	if b.Percent() != 45 {
		t.Fatalf("percent = %d, want 45", b.Percent())
	}
}

func TestParseBrightnessVerbose(t *testing.T) {
	out := "VCP code 0x10 (Brightness): current value =    40, max value =   100\n"
	b, err := parseBrightness(out)
	if err != nil {
		t.Fatal(err)
	}
	if b.Current != 40 || b.Max != 100 {
		t.Fatalf("got %+v", b)
	}
}

func TestParseDetectOutput(t *testing.T) {
	out := "Display 1\n   I2C bus:          /dev/i2c-6\n   Monitor:          DEL:AW2724DM\n\n" +
		"Display 2\n   I2C bus: /dev/i2c-8\n\nDisplay 3\n   I2C bus:  /dev/i2c-9\n"

	displays := parseDetectOutput(out)
	if len(displays) != 3 {
		t.Fatalf("displays = %d, want 3", len(displays))
	}
	want := []Display{{Number: 1, Bus: 6}, {Number: 2, Bus: 8}, {Number: 3, Bus: 9}}
	for i, d := range displays {
		if d != want[i] {
			t.Fatalf("display[%d] = %+v, want %+v", i, d, want[i])
		}
	}
}

func TestStripKnownNoise(t *testing.T) {
	stderr := "Device /dev/i2c-0 is not readable and writable.  Error = EACCES(13): Permission denied\n" +
		"Devices possibly used for DDC/CI communication cannot be opened: /dev/i2c-0\n" +
		"See https://www.ddcutil.com/i2c_permissions\n" +
		"Display not found\n"

	cleaned := stripKnownNoise(stderr)
	if cleaned != "Display not found" {
		t.Fatalf("cleaned = %q, want %q", cleaned, "Display not found")
	}
}

func TestIsOnlyI2C0Noise(t *testing.T) {
	stderr := "Device /dev/i2c-0 is not readable and writable.  Error = EACCES(13): Permission denied\n" +
		"Devices possibly used for DDC/CI communication cannot be opened: /dev/i2c-0\n" +
		"See https://www.ddcutil.com/i2c_permissions\n"

	if !IsOnlyI2C0Noise(stderr) {
		t.Fatal("expected only i2c-0 noise")
	}
	if IsOnlyI2C0Noise(stderr + "Display not found\n") {
		t.Fatal("expected mixed stderr to not be only i2c-0 noise")
	}
}

func TestProbeI2C0NoiseLive(t *testing.T) {
	c := NewClient("ddcutil", defaultTimeout, false)
	_, stderr, err := c.runOnce(context.Background(), "detect", "--brief")
	if err != nil {
		t.Skipf("ddcutil detect failed: %v", err)
	}
	t.Logf("stderr=%q", stderr)
	t.Logf("IsOnlyI2C0Noise=%v ProbeI2C0Noise=%v", IsOnlyI2C0Noise(stderr), c.ProbeI2C0Noise(context.Background()))
}
