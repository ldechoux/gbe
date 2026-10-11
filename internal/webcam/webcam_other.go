//go:build !darwin && !linux && !windows

package webcam

// Devices lists the cameras of the system.
func Devices() ([]Device, error) { return nil, ErrUnsupported }

// Open starts the camera id ("" for the default one).
func Open(id string) (*Stream, error) { return nil, ErrUnsupported }
