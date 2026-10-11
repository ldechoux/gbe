package webcam

import (
	"bytes"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"unsafe"
)

// The structures have the sizes of linux/videodev2.h on 64-bit systems:
// the ioctl numbers hold them.
func TestV4L2Sizes(t *testing.T) {
	for _, c := range []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"v4l2_capability", unsafe.Sizeof(v4l2Capability{}), 104},
		{"v4l2_fmtdesc", unsafe.Sizeof(v4l2Fmtdesc{}), 64},
		{"v4l2_format", unsafe.Sizeof(v4l2Format{}), 208},
		{"v4l2_requestbuffers", unsafe.Sizeof(v4l2Requestbuffers{}), 20},
		{"v4l2_buffer", unsafe.Sizeof(v4l2Buffer{}), 88},
	} {
		if c.got != c.want {
			t.Errorf("%s: %d bytes, want %d", c.name, c.got, c.want)
		}
	}
	// As the kernel headers give them.
	for _, c := range []struct {
		name      string
		got, want uintptr
	}{
		{"VIDIOC_QUERYCAP", vidiocQuerycap, 0x80685600},
		{"VIDIOC_S_FMT", vidiocSFmt, 0xc0d05605},
		{"VIDIOC_REQBUFS", vidiocReqbufs, 0xc0145608},
		{"VIDIOC_QBUF", vidiocQbuf, 0xc058560f},
		{"VIDIOC_DQBUF", vidiocDqbuf, 0xc0585611},
		{"VIDIOC_STREAMON", vidiocStreamon, 0x40045612},
	} {
		if c.got != c.want {
			t.Errorf("%s = %#x, want %#x", c.name, c.got, c.want)
		}
	}
	if fmtYUYV != 0x56595559 {
		t.Errorf("YUYV = %#x", fmtYUYV)
	}
}

// The luma of each format: a 4x2 picture whose pixels are 10, 20, ...
func TestKeepFormats(t *testing.T) {
	want := []byte{10, 20, 30, 40, 50, 60, 70, 80}
	packed := func(lumaFirst bool) []byte { // 4:2:2, 10 bytes per row
		var out []byte
		for y := range 2 {
			for x := 0; x < 4; x += 2 {
				a, b := want[y*4+x], want[y*4+x+1]
				if lumaFirst {
					out = append(out, a, 128, b, 128)
				} else {
					out = append(out, 128, a, 128, b)
				}
			}
			out = append(out, 0, 0) // padding
		}
		return out
	}
	planar := append([]byte{10, 20, 30, 40, 0, 50, 60, 70, 80, 0}, 128, 128, 128, 128) // stride 5, then chroma
	for _, c := range []struct {
		name   string
		format uint32
		stride uint32
		data   []byte
	}{
		{"GREY", fmtGrey, 5, planar[:10]},
		{"NV12", fmtNV12, 5, planar},
		{"YUYV", fmtYUYV, 10, packed(true)},
		{"UYVY", fmtUYVY, 10, packed(false)},
	} {
		cam := &v4l2Camera{format: v4l2PixFormat{width: 4, height: 2, pixelformat: c.format, bytesperline: c.stride}}
		var s Stream
		cam.keep(&s, c.data)
		var img *image.Gray
		if !s.Frame(&img) || !bytes.Equal(img.Pix, want) {
			t.Errorf("%s: %v", c.name, img)
		}
		// A picture cut short is left out.
		var short Stream
		cam.keep(&short, c.data[:len(c.data)/2])
		if short.Frames() != 0 {
			t.Errorf("%s: a short picture was kept", c.name)
		}
	}
}

// MJPEG pictures are decoded, with the standard Huffman tables when they
// leave them out, as many webcams do.
func TestKeepMJPEG(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 16, 16))
	for i := range src.Pix {
		src.Pix[i] = 200
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	full := buf.Bytes()
	// Without its tables: the DHT segment cut out.
	i := bytes.Index(full, []byte{0xFF, 0xC4})
	n := int(full[i+2])<<8 | int(full[i+3])
	stripped := append(append([]byte{}, full[:i]...), full[i+2+n:]...)
	if _, err := jpeg.Decode(bytes.NewReader(stripped)); err == nil {
		t.Fatal("the decoder reads a JPEG without its tables")
	}
	if !bytes.Equal(withHuffman(full), full) {
		t.Error("tables added to a JPEG that has them")
	}
	for _, data := range [][]byte{full, stripped} {
		cam := &v4l2Camera{format: v4l2PixFormat{width: 16, height: 16, pixelformat: fmtMJPEG}}
		var s Stream
		cam.keep(&s, data)
		var img *image.Gray
		if !s.Frame(&img) || img.Rect.Dx() != 16 || img.GrayAt(8, 8).Y < 195 || img.GrayAt(8, 8).Y > 205 {
			t.Errorf("MJPEG of %d bytes: %v", len(data), img)
		}
	}
	var s Stream
	(&v4l2Camera{format: v4l2PixFormat{pixelformat: fmtMJPEG}}).keep(&s, []byte("not a jpeg"))
	if s.Frames() != 0 {
		t.Error("a broken JPEG was kept")
	}
}

// Without a camera, or for a camera that is gone, Open says so; the files
// that are not cameras are left out.
func TestDevicesWithoutCamera(t *testing.T) {
	old := devDir
	defer func() { devDir = old }()
	devDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(devDir, "video0"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	devices, err := Devices()
	if err != nil || len(devices) != 0 {
		t.Errorf("devices %v, %v", devices, err)
	}
	if _, err := Open(""); err != ErrNoDevice {
		t.Errorf("Open without a camera: %v", err)
	}
	if _, err := Open(filepath.Join(devDir, "video9")); err != ErrNoDevice {
		t.Errorf("Open of a camera gone: %v", err)
	}
	if !nodeLess("/dev/video2", "/dev/video10") || nodeLess("/dev/video10", "/dev/video9") {
		t.Error("video10 sorted before video2")
	}
}
