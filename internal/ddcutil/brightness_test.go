package ddcutil

import "testing"

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

func TestDetectDisplayLines(t *testing.T) {
	out := "Display 1\n   I2C bus: /dev/i2c-6\n\nDisplay 2\n   I2C bus: /dev/i2c-8\n"
	matches := displayLineRE.FindAllStringSubmatch(out, -1)
	if len(matches) != 2 {
		t.Fatalf("matches = %d, want 2", len(matches))
	}
}
