package webcam

// Linux: Video4Linux2, through its ioctls on /dev/video*. The camera
// fills buffers the kernel maps in memory (streaming I/O); a goroutine
// takes each picture and gives it back. Of its formats, the ones whose
// luma reads directly come first: gray, then YUYV and its kin, then
// NV12; MJPEG, the last resort, is decoded.

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"path/filepath"
	"sort"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

// The ioctls and constants of linux/videodev2.h used here, for the 64-bit
// systems gbe is built for.
const (
	capVideoCapture = 0x00000001
	capStreaming    = 0x04000000
	capDeviceCaps   = 0x80000000

	bufTypeVideoCapture = 1
	memoryMmap          = 1
	fieldAny            = 0

	numBuffers = 4
)

// ioc is _IOC of linux/ioctl.h: the direction (1 write, 2 read), the type
// 'V', the number and the size of the argument.
func ioc(dir, nr, size uintptr) uintptr { return dir<<30 | size<<16 | 'V'<<8 | nr }

var (
	vidiocQuerycap  = ioc(2, 0, unsafe.Sizeof(v4l2Capability{}))
	vidiocEnumFmt   = ioc(3, 2, unsafe.Sizeof(v4l2Fmtdesc{}))
	vidiocSFmt      = ioc(3, 5, unsafe.Sizeof(v4l2Format{}))
	vidiocReqbufs   = ioc(3, 8, unsafe.Sizeof(v4l2Requestbuffers{}))
	vidiocQuerybuf  = ioc(3, 9, unsafe.Sizeof(v4l2Buffer{}))
	vidiocQbuf      = ioc(3, 15, unsafe.Sizeof(v4l2Buffer{}))
	vidiocDqbuf     = ioc(3, 17, unsafe.Sizeof(v4l2Buffer{}))
	vidiocStreamon  = ioc(1, 18, unsafe.Sizeof(int32(0)))
	vidiocStreamoff = ioc(1, 19, unsafe.Sizeof(int32(0)))
)

// fourcc is a pixel format code of V4L2.
func fourcc(s string) uint32 {
	return uint32(s[0]) | uint32(s[1])<<8 | uint32(s[2])<<16 | uint32(s[3])<<24
}

// The pixel formats read, the best first.
var (
	fmtGrey  = fourcc("GREY")
	fmtYUYV  = fourcc("YUYV")
	fmtYVYU  = fourcc("YVYU")
	fmtUYVY  = fourcc("UYVY")
	fmtNV12  = fourcc("NV12")
	fmtNV21  = fourcc("NV21")
	fmtMJPEG = fourcc("MJPG")
	formats  = []uint32{fmtGrey, fmtYUYV, fmtYVYU, fmtUYVY, fmtNV12, fmtNV21, fmtMJPEG}
)

type v4l2Capability struct {
	driver       [16]byte
	card         [32]byte
	busInfo      [32]byte
	version      uint32
	capabilities uint32
	deviceCaps   uint32
	reserved     [3]uint32
}

type v4l2Fmtdesc struct {
	index       uint32
	typ         uint32
	flags       uint32
	description [32]byte
	pixelformat uint32
	mbusCode    uint32
	reserved    [3]uint32
}

type v4l2PixFormat struct {
	width        uint32
	height       uint32
	pixelformat  uint32
	field        uint32
	bytesperline uint32
	sizeimage    uint32
	colorspace   uint32
	priv         uint32
	flags        uint32
	ycbcrEnc     uint32
	quantization uint32
	xferFunc     uint32
}

// v4l2Format is struct v4l2_format: its union of 200 bytes starts at 8,
// as one of its members holds pointers.
type v4l2Format struct {
	typ uint32
	_   uint32
	pix v4l2PixFormat
	_   [200 - unsafe.Sizeof(v4l2PixFormat{})]byte
}

type v4l2Requestbuffers struct {
	count        uint32
	typ          uint32
	memory       uint32
	capabilities uint32
	flags        uint8
	reserved     [3]uint8
}

// v4l2Buffer is struct v4l2_buffer, with a 64-bit struct timeval.
type v4l2Buffer struct {
	index     uint32
	typ       uint32
	bytesused uint32
	flags     uint32
	field     uint32
	_         uint32
	timestamp [2]int64
	timecode  [16]byte
	sequence  uint32
	memory    uint32
	offset    uint64 // union m: the offset of the buffer to map
	length    uint32
	reserved2 uint32
	requestFD int32
	_         uint32
}

func ioctl(fd int, req uintptr, arg unsafe.Pointer) error {
	for {
		_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), req, uintptr(arg))
		switch errno {
		case 0:
			return nil
		case unix.EINTR:
			continue
		}
		return errno
	}
}

// cString is a NUL-terminated field of a V4L2 structure.
func cString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return strings.TrimSpace(string(b))
}

// capture reads the capabilities of the device at path, and reports
// whether it captures video by streaming.
func capture(path string) (v4l2Capability, bool) {
	var c v4l2Capability
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return c, false
	}
	defer unix.Close(fd)
	if ioctl(fd, vidiocQuerycap, unsafe.Pointer(&c)) != nil {
		return c, false
	}
	caps := c.capabilities
	if caps&capDeviceCaps != 0 {
		caps = c.deviceCaps // the ones of this node, not of the whole driver
	}
	return c, caps&capVideoCapture != 0 && caps&capStreaming != 0
}

// devDir is where the device nodes are, a variable for the tests.
var devDir = "/dev"

// Devices lists the cameras of the system. Their ID is their path under
// /dev/v4l/by-id when there is one, which stays the same from a boot to
// the next, or else their node.
func Devices() ([]Device, error) {
	nodes, _ := filepath.Glob(filepath.Join(devDir, "video*"))
	sort.Slice(nodes, func(i, j int) bool { return nodeLess(nodes[i], nodes[j]) })
	byID := map[string]string{} // node -> its by-id link
	links, _ := filepath.Glob(filepath.Join(devDir, "v4l", "by-id", "*"))
	for _, l := range links {
		if node, err := filepath.EvalSymlinks(l); err == nil {
			if _, ok := byID[node]; !ok || l < byID[node] {
				byID[node] = l
			}
		}
	}
	var devices []Device
	for _, node := range nodes {
		c, ok := capture(node)
		if !ok {
			continue // a metadata node, or not a camera
		}
		id := node
		if l, ok := byID[node]; ok {
			id = l
		}
		devices = append(devices, Device{ID: id, Name: cString(c.card[:])})
	}
	return devices, nil
}

// nodeLess sorts video2 before video10.
func nodeLess(a, b string) bool {
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return a < b
}

// Open starts the camera id ("" for the first one).
func Open(id string) (*Stream, error) {
	path := id
	if path == "" {
		devices, _ := Devices()
		if len(devices) == 0 {
			return nil, ErrNoDevice
		}
		path = devices[0].ID
	}
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	switch {
	case errors.Is(err, unix.EACCES), errors.Is(err, unix.EPERM):
		return nil, fmt.Errorf("%w: %s (is the user in the video group?)", ErrDenied, path)
	case errors.Is(err, unix.ENOENT), errors.Is(err, unix.ENODEV), errors.Is(err, unix.ENXIO):
		return nil, ErrNoDevice
	case err != nil:
		return nil, fmt.Errorf("webcam: %s: %w", path, err)
	}
	cam := &v4l2Camera{fd: fd}
	if err := cam.start(); err != nil {
		cam.release()
		return nil, fmt.Errorf("webcam: %s: %w", path, err)
	}
	s := &Stream{}
	stop, done := make(chan struct{}), make(chan struct{})
	go cam.run(s, stop, done)
	s.closer = func() {
		close(stop)
		<-done
		cam.release()
	}
	return s, nil
}

// v4l2Camera is an open device and its buffers.
type v4l2Camera struct {
	fd        int
	format    v4l2PixFormat
	buffers   [][]byte
	streaming bool
}

// start chooses the format, maps the buffers and starts the capture.
func (c *v4l2Camera) start() error {
	pixfmt, err := c.chooseFormat()
	if err != nil {
		return err
	}
	f := v4l2Format{typ: bufTypeVideoCapture}
	f.pix = v4l2PixFormat{width: wantWidth, height: wantHeight, pixelformat: pixfmt, field: fieldAny}
	if err := ioctl(c.fd, vidiocSFmt, unsafe.Pointer(&f)); err != nil {
		return fmt.Errorf("setting the format: %w", err)
	}
	c.format = f.pix // what the driver chose
	if c.format.pixelformat != pixfmt || c.format.width == 0 || c.format.height == 0 {
		return fmt.Errorf("the camera gives %dx%d pictures in format %#x", c.format.width, c.format.height, c.format.pixelformat)
	}
	if c.format.bytesperline == 0 {
		c.format.bytesperline = c.format.width * bytesPerPixel(pixfmt)
	}

	req := v4l2Requestbuffers{count: numBuffers, typ: bufTypeVideoCapture, memory: memoryMmap}
	if err := ioctl(c.fd, vidiocReqbufs, unsafe.Pointer(&req)); err != nil {
		return fmt.Errorf("asking for buffers: %w", err)
	}
	if req.count == 0 {
		return errors.New("no buffers")
	}
	for i := range req.count {
		b := v4l2Buffer{index: i, typ: bufTypeVideoCapture, memory: memoryMmap}
		if err := ioctl(c.fd, vidiocQuerybuf, unsafe.Pointer(&b)); err != nil {
			return fmt.Errorf("buffer %d: %w", i, err)
		}
		mem, err := unix.Mmap(c.fd, int64(b.offset), int(b.length), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
		if err != nil {
			return fmt.Errorf("mapping buffer %d: %w", i, err)
		}
		c.buffers = append(c.buffers, mem)
		if err := ioctl(c.fd, vidiocQbuf, unsafe.Pointer(&b)); err != nil {
			return fmt.Errorf("queuing buffer %d: %w", i, err)
		}
	}
	typ := int32(bufTypeVideoCapture)
	if err := ioctl(c.fd, vidiocStreamon, unsafe.Pointer(&typ)); err != nil {
		return fmt.Errorf("starting the capture: %w", err)
	}
	c.streaming = true
	return nil
}

// chooseFormat is the best format the camera offers (see formats).
func (c *v4l2Camera) chooseFormat() (uint32, error) {
	offered := map[uint32]bool{}
	var names []string
	for i := uint32(0); ; i++ {
		d := v4l2Fmtdesc{index: i, typ: bufTypeVideoCapture}
		if ioctl(c.fd, vidiocEnumFmt, unsafe.Pointer(&d)) != nil {
			break
		}
		offered[d.pixelformat] = true
		names = append(names, cString(d.description[:]))
	}
	for _, f := range formats {
		if offered[f] {
			return f, nil
		}
	}
	return 0, fmt.Errorf("no format gbe reads among %q", names)
}

func bytesPerPixel(pixfmt uint32) uint32 {
	switch pixfmt {
	case fmtYUYV, fmtYVYU, fmtUYVY:
		return 2
	}
	return 1
}

// run takes the pictures until stop is closed.
func (c *v4l2Camera) run(s *Stream, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	poll := []unix.PollFd{{Fd: int32(c.fd), Events: unix.POLLIN}}
	for {
		select {
		case <-stop:
			return
		default:
		}
		// A short wait, to see stop soon.
		n, err := unix.Poll(poll, 100)
		if errors.Is(err, unix.EINTR) || err == nil && n == 0 {
			continue
		}
		if err != nil {
			return
		}
		b := v4l2Buffer{typ: bufTypeVideoCapture, memory: memoryMmap}
		switch err := ioctl(c.fd, vidiocDqbuf, unsafe.Pointer(&b)); {
		case errors.Is(err, unix.EAGAIN):
			continue
		case err != nil:
			return // unplugged
		}
		if int(b.index) < len(c.buffers) {
			data := c.buffers[b.index]
			c.keep(s, data[:min(int(b.bytesused), len(data))])
		}
		if ioctl(c.fd, vidiocQbuf, unsafe.Pointer(&b)) != nil {
			return
		}
	}
}

// keep gives s the luma of a picture in the format of the camera.
func (c *v4l2Camera) keep(s *Stream, data []byte) {
	w, h, stride := int(c.format.width), int(c.format.height), int(c.format.bytesperline)
	switch c.format.pixelformat {
	case fmtGrey, fmtNV12, fmtNV21:
		if len(data) >= stride*(h-1)+w {
			s.put(data, w, h, stride)
		}
	case fmtYUYV, fmtYVYU, fmtUYVY:
		first := 0 // where the luma of a pair of pixels is
		if c.format.pixelformat == fmtUYVY {
			first = 1
		}
		if len(data) < stride*(h-1)+2*w {
			return
		}
		luma := make([]byte, w*h)
		for y := range h {
			row := data[y*stride:]
			for x := range w {
				luma[y*w+x] = row[2*x+first]
			}
		}
		s.put(luma, w, h, w)
	case fmtMJPEG:
		img, err := jpeg.Decode(bytes.NewReader(withHuffman(data)))
		if err != nil {
			return // a picture cut short
		}
		if ycc, ok := img.(*image.YCbCr); ok {
			r := ycc.Rect
			s.put(ycc.Y[ycc.YOffset(r.Min.X, r.Min.Y):], r.Dx(), r.Dy(), ycc.YStride)
		} else if g, ok := img.(*image.Gray); ok {
			s.put(g.Pix, g.Rect.Dx(), g.Rect.Dy(), g.Stride)
		}
	}
}

// release stops the capture and frees the buffers and the device.
func (c *v4l2Camera) release() {
	if c.streaming {
		typ := int32(bufTypeVideoCapture)
		ioctl(c.fd, vidiocStreamoff, unsafe.Pointer(&typ))
		c.streaming = false
	}
	for _, b := range c.buffers {
		unix.Munmap(b)
	}
	c.buffers = nil
	if c.fd >= 0 {
		unix.Close(c.fd)
		c.fd = -1
	}
}

// standardDHT is the segment of the Huffman tables of the JPEG standard
// (annex K), which the MJPEG pictures of many webcams leave out: the one
// the encoder of Go writes.
var standardDHT = func() []byte {
	var buf bytes.Buffer
	jpeg.Encode(&buf, image.NewGray(image.Rect(0, 0, 8, 8)), nil)
	data := buf.Bytes()
	i := bytes.Index(data, []byte{0xFF, 0xC4})
	if i < 0 || i+4 > len(data) {
		return nil
	}
	n := int(data[i+2])<<8 | int(data[i+3])
	return data[i : i+2+n]
}()

// withHuffman gives a JPEG without Huffman tables the standard ones, before
// its scan.
func withHuffman(data []byte) []byte {
	sos := bytes.Index(data, []byte{0xFF, 0xDA})
	if sos < 0 || bytes.Contains(data[:sos], []byte{0xFF, 0xC4}) {
		return data
	}
	out := make([]byte, 0, len(data)+len(standardDHT))
	out = append(out, data[:sos]...)
	out = append(out, standardDHT...)
	return append(out, data[sos:]...)
}
