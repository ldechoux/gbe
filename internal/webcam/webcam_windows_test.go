package webcam

import (
	"errors"
	"os"
	"testing"
)

// The GUIDs made from a code are those of mfapi.h.
func TestMediaTypeGUIDs(t *testing.T) {
	for _, c := range []struct {
		name string
		got  string
		want string
	}{
		{"MFMediaType_Video", mediaTypeVideo.String(), "{73646976-0000-0010-8000-00AA00389B71}"},
		{"MFVideoFormat_NV12", formatNV12.String(), "{3231564E-0000-0010-8000-00AA00389B71}"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %s, want %s", c.name, c.got, c.want)
		}
	}
}

// A failed HRESULT is an error, access denied ErrDenied.
func TestCheck(t *testing.T) {
	if check("ok", 0) != nil || check("ok", 1) != nil {
		t.Error("S_OK and S_FALSE fail")
	}
	if err := check("Activate", eAccessDenied); !errors.Is(err, ErrDenied) {
		t.Errorf("access denied: %v", err)
	}
	if err := check("Read", 0xC00D36B9); err == nil || errors.Is(err, ErrDenied) || err.Error() != "webcam: Read: HRESULT 0xc00d36b9" {
		t.Errorf("another failure: %v", err)
	}
}

// Media Foundation lists the cameras, none on a machine without (the CI
// sets GBE_WEBCAM_LIST): the calls through COM work.
func TestDevicesList(t *testing.T) {
	if os.Getenv("GBE_WEBCAM_LIST") == "" {
		t.Skip("set GBE_WEBCAM_LIST=1 to list the cameras")
	}
	devices, err := Devices()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d cameras: %v", len(devices), devices)
	if len(devices) == 0 {
		if _, err := Open(""); !errors.Is(err, ErrNoDevice) {
			t.Errorf("Open without a camera: %v", err)
		}
	}
}
