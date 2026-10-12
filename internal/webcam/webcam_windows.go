package webcam

// Windows: Media Foundation, through its COM interfaces called from Go
// (syscall.SyscallN on their method tables). A source reader turns what
// the camera gives (MJPEG, YUY2...) into NV12, whose first plane is the
// luma; a goroutine reads it, on a thread of its own, where COM lives.

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ole32       = windows.NewLazySystemDLL("ole32.dll")
	mfplat      = windows.NewLazySystemDLL("mfplat.dll")
	mf          = windows.NewLazySystemDLL("mf.dll")
	mfreadwrite = windows.NewLazySystemDLL("mfreadwrite.dll")

	procCoInitializeEx = ole32.NewProc("CoInitializeEx")
	procCoUninitialize = ole32.NewProc("CoUninitialize")
	procCoTaskMemFree  = ole32.NewProc("CoTaskMemFree")
	procMFStartup      = mfplat.NewProc("MFStartup")
	procMFShutdown     = mfplat.NewProc("MFShutdown")
	procMFCreateAttrs  = mfplat.NewProc("MFCreateAttributes")
	procMFCreateType   = mfplat.NewProc("MFCreateMediaType")
	procMFEnumDevices  = mf.NewProc("MFEnumDeviceSources")
	procMFCreateReader = mfreadwrite.NewProc("MFCreateSourceReaderFromMediaSource")
)

const (
	coinitMultithreaded = 0x0
	mfVersion           = 0x00020070 // MF_VERSION of Windows 7 and later
	mfStartupFull       = 0

	firstVideoStream = 0xFFFFFFFC // MF_SOURCE_READER_FIRST_VIDEO_STREAM
	readerfError     = 0x1        // MF_SOURCE_READERF_ERROR
	readerfEnd       = 0x2        // MF_SOURCE_READERF_ENDOFSTREAM

	// The width of picture asked for, among those the camera offers:
	// enough for the 128x112 of the Game Boy Camera, and light to convert.
	wantWidth = 640

	eAccessDenied   = 0x80070005
	rpcEChangedMode = 0x80010106 // COM already set up otherwise on the thread
)

// Methods, by their place in the tables of the interfaces (Wine's
// mfobjects.idl, mfidl.idl and mfreadwrite.idl): IUnknown has 3, and
// IMFAttributes 30 more, which IMFActivate, IMFMediaType and IMFSample
// start with.
const (
	mRelease = 2

	mGetUINT64          = 8
	mGetAllocatedString = 13
	mSetUINT32          = 21
	mSetGUID            = 24

	mActivateObject = 33 // IMFActivate

	mShutdown = 12 // IMFMediaSource, after IMFMediaEventGenerator

	mGetNativeMediaType  = 5 // IMFSourceReader
	mSetCurrentMediaType = 7
	mGetCurrentMediaType = 6
	mReadSample          = 9

	mConvertToContiguous = 41 // IMFSample

	mLock   = 3 // IMFMediaBuffer
	mUnlock = 4
)

var (
	guidSourceType      = windows.GUID{Data1: 0xc60ac5fe, Data2: 0x252a, Data3: 0x478f, Data4: [8]byte{0xa0, 0xef, 0xbc, 0x8f, 0xa5, 0xf7, 0xca, 0xd3}}
	guidSourceVidcap    = windows.GUID{Data1: 0x8ac3587a, Data2: 0x4ae7, Data3: 0x42d8, Data4: [8]byte{0x99, 0xe0, 0x0a, 0x60, 0x13, 0xee, 0xf9, 0x0f}}
	guidFriendlyName    = windows.GUID{Data1: 0x60d0e559, Data2: 0x52f8, Data3: 0x4fa2, Data4: [8]byte{0xbb, 0xce, 0xac, 0xdb, 0x34, 0xa8, 0xec, 0x01}}
	guidSymbolicLink    = windows.GUID{Data1: 0x58f0aad8, Data2: 0x22bf, Data3: 0x4f8a, Data4: [8]byte{0xbb, 0x3d, 0xd2, 0xc4, 0x97, 0x8c, 0x6e, 0x2f}}
	guidVideoProcessing = windows.GUID{Data1: 0xfb394f3d, Data2: 0xccf1, Data3: 0x42ee, Data4: [8]byte{0xbb, 0xb3, 0xf9, 0xb8, 0x45, 0xd5, 0x68, 0x1d}}
	guidMajorType       = windows.GUID{Data1: 0x48eba18e, Data2: 0xf8c9, Data3: 0x4687, Data4: [8]byte{0xbf, 0x11, 0x0a, 0x74, 0xc9, 0xf9, 0x6a, 0x8f}}
	guidSubtype         = windows.GUID{Data1: 0xf7e34c9a, Data2: 0x42e8, Data3: 0x4714, Data4: [8]byte{0xb7, 0x4b, 0xcb, 0x29, 0xd7, 0x2c, 0x35, 0xe5}}
	guidFrameSize       = windows.GUID{Data1: 0x1652c33d, Data2: 0xd6b2, Data3: 0x4012, Data4: [8]byte{0xb8, 0x34, 0x72, 0x03, 0x08, 0x49, 0xa3, 0x7d}}
	iidIMFMediaSource   = windows.GUID{Data1: 0x279a808d, Data2: 0xaec7, Data3: 0x40c8, Data4: [8]byte{0x9c, 0x6b, 0xa6, 0xb4, 0x92, 0xc7, 0x8a, 0x66}}

	mediaTypeVideo = videoFormat(0x73646976) // 'vids'
	formatNV12     = videoFormat(0x3231564E) // 'NV12'
)

// videoFormat is the GUID of a media type or a video format from its code:
// DEFINE_MEDIATYPE_GUID of mfapi.h.
func videoFormat(code uint32) windows.GUID {
	return windows.GUID{Data1: code, Data2: 0x0000, Data3: 0x0010, Data4: [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}
}

// com is a COM object: a pointer to its method table.
type com struct{ vtbl *[64]uintptr }

// call calls method m of o.
func (o *com) call(m int, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(o.vtbl[m], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return r
}

func (o *com) release() {
	if o != nil {
		o.call(mRelease)
	}
}

// hresult is the error of a failed HRESULT.
type hresult uint32

func (h hresult) Error() string { return fmt.Sprintf("HRESULT %#08x", uint32(h)) }

// check turns an HRESULT into an error: negative ones fail.
func check(what string, r uintptr) error {
	if int32(r) >= 0 {
		return nil
	}
	if uint32(r) == eAccessDenied {
		return fmt.Errorf("%w: %s (allow the desktop apps in the privacy settings of the camera)", ErrDenied, what)
	}
	return fmt.Errorf("webcam: %s: %w", what, hresult(r))
}

// onCOMThread runs fn on a thread of its own, set up for COM and Media
// Foundation, and returns its error.
func onCOMThread(fn func() error) error {
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread() // the thread ends with the goroutine
		done <- withCOM(fn)
	}()
	return <-done
}

// withCOM sets up COM and Media Foundation on the current thread, locked,
// for fn.
func withCOM(fn func() error) error {
	if err := mfplat.Load(); err != nil {
		return fmt.Errorf("%w: no Media Foundation (%v)", ErrUnsupported, err)
	}
	r, _, _ := procCoInitializeEx.Call(0, coinitMultithreaded)
	if int32(r) < 0 && uint32(r) != rpcEChangedMode {
		return check("CoInitializeEx", r)
	}
	if int32(r) >= 0 {
		defer procCoUninitialize.Call()
	}
	r, _, _ = procMFStartup.Call(mfVersion, mfStartupFull)
	if err := check("MFStartup", r); err != nil {
		return err
	}
	defer procMFShutdown.Call()
	return fn()
}

// enumDevices lists the video capture devices; the caller releases them
// and frees the list.
func enumDevices() ([]*com, unsafe.Pointer, error) {
	var attrs *com
	r, _, _ := procMFCreateAttrs.Call(uintptr(unsafe.Pointer(&attrs)), 1)
	if err := check("MFCreateAttributes", r); err != nil {
		return nil, nil, err
	}
	defer attrs.release()
	if err := check("SetGUID", attrs.call(mSetGUID, uintptr(unsafe.Pointer(&guidSourceType)), uintptr(unsafe.Pointer(&guidSourceVidcap)))); err != nil {
		return nil, nil, err
	}
	var list unsafe.Pointer
	var count uint32
	r, _, _ = procMFEnumDevices.Call(uintptr(unsafe.Pointer(attrs)), uintptr(unsafe.Pointer(&list)), uintptr(unsafe.Pointer(&count)))
	if err := check("MFEnumDeviceSources", r); err != nil {
		return nil, nil, err
	}
	if count == 0 || list == nil {
		return nil, list, nil
	}
	return unsafe.Slice((**com)(list), count), list, nil
}

func freeDevices(devices []*com, list unsafe.Pointer) {
	for _, d := range devices {
		d.release()
	}
	if list != nil {
		procCoTaskMemFree.Call(uintptr(list))
	}
}

// stringAttr reads a string attribute of o.
func stringAttr(o *com, key *windows.GUID) string {
	var p *uint16
	var n uint32
	if int32(o.call(mGetAllocatedString, uintptr(unsafe.Pointer(key)), uintptr(unsafe.Pointer(&p)), uintptr(unsafe.Pointer(&n)))) < 0 || p == nil {
		return ""
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(p)))
	return windows.UTF16PtrToString(p)
}

// Devices lists the cameras of the system. Their ID is their symbolic
// link, which stays the same from a launch to the next.
func Devices() ([]Device, error) {
	var out []Device
	err := onCOMThread(func() error {
		devices, list, err := enumDevices()
		if err != nil {
			return err
		}
		defer freeDevices(devices, list)
		for _, d := range devices {
			out = append(out, Device{ID: stringAttr(d, &guidSymbolicLink), Name: stringAttr(d, &guidFriendlyName)})
		}
		return nil
	})
	return out, err
}

// Open starts the camera id ("" for the first one).
func Open(id string) (*Stream, error) {
	s := &Stream{}
	opened := make(chan error, 1)
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		err := withCOM(func() error {
			reader, source, err := openReader(id)
			if err != nil {
				return err
			}
			defer func() {
				reader.release()
				source.call(mShutdown)
				source.release()
			}()
			width, height, err := frameSize(reader)
			if err != nil {
				return err
			}
			opened <- nil
			readSamples(reader, s, width, height, stop)
			return nil
		})
		if err != nil {
			opened <- err
		}
	}()
	if err := <-opened; err != nil {
		<-done
		return nil, err
	}
	s.closer = func() {
		close(stop)
		<-done
	}
	return s, nil
}

// openReader activates the camera id and makes it a source reader of NV12
// pictures, of the size closest to 640 pixels wide it offers.
func openReader(id string) (reader, source *com, err error) {
	devices, list, err := enumDevices()
	if err != nil {
		return nil, nil, err
	}
	defer freeDevices(devices, list)
	var device *com
	for _, d := range devices {
		if id == "" || stringAttr(d, &guidSymbolicLink) == id {
			device = d
			break
		}
	}
	if device == nil {
		return nil, nil, ErrNoDevice
	}
	if err := check("ActivateObject", device.call(mActivateObject, uintptr(unsafe.Pointer(&iidIMFMediaSource)), uintptr(unsafe.Pointer(&source)))); err != nil {
		return nil, nil, err
	}

	var attrs *com
	r, _, _ := procMFCreateAttrs.Call(uintptr(unsafe.Pointer(&attrs)), 1)
	if err := check("MFCreateAttributes", r); err != nil {
		source.release()
		return nil, nil, err
	}
	defer attrs.release()
	attrs.call(mSetUINT32, uintptr(unsafe.Pointer(&guidVideoProcessing)), 1) // converts to NV12
	r, _, _ = procMFCreateReader.Call(uintptr(unsafe.Pointer(source)), uintptr(unsafe.Pointer(attrs)), uintptr(unsafe.Pointer(&reader)))
	if err := check("MFCreateSourceReaderFromMediaSource", r); err != nil {
		source.call(mShutdown)
		source.release()
		return nil, nil, err
	}

	chooseNativeType(reader)
	var out *com
	r, _, _ = procMFCreateType.Call(uintptr(unsafe.Pointer(&out)))
	if err = check("MFCreateMediaType", r); err == nil {
		defer out.release()
		out.call(mSetGUID, uintptr(unsafe.Pointer(&guidMajorType)), uintptr(unsafe.Pointer(&mediaTypeVideo)))
		out.call(mSetGUID, uintptr(unsafe.Pointer(&guidSubtype)), uintptr(unsafe.Pointer(&formatNV12)))
		err = check("SetCurrentMediaType", reader.call(mSetCurrentMediaType, firstVideoStream, 0, uintptr(unsafe.Pointer(out))))
	}
	if err != nil {
		reader.release()
		source.call(mShutdown)
		source.release()
		return nil, nil, err
	}
	return reader, source, nil
}

// chooseNativeType sets the camera to the format it offers whose width is
// the closest to 640: enough for the 128x112 of the Game Boy Camera, and
// light to convert.
func chooseNativeType(reader *com) {
	var best *com
	bestDist := -1
	for i := uintptr(0); ; i++ {
		var t *com
		if int32(reader.call(mGetNativeMediaType, firstVideoStream, i, uintptr(unsafe.Pointer(&t)))) < 0 {
			break
		}
		var size uint64
		if int32(t.call(mGetUINT64, uintptr(unsafe.Pointer(&guidFrameSize)), uintptr(unsafe.Pointer(&size)))) < 0 {
			t.release()
			continue
		}
		dist := int(size>>32) - wantWidth
		if dist < 0 {
			dist = -dist
		}
		if bestDist < 0 || dist < bestDist {
			best.release()
			best, bestDist = t, dist
		} else {
			t.release()
		}
	}
	if best != nil {
		reader.call(mSetCurrentMediaType, firstVideoStream, 0, uintptr(unsafe.Pointer(best)))
		best.release()
	}
}

// frameSize is the size of the pictures the reader gives.
func frameSize(reader *com) (w, h int, err error) {
	var t *com
	if err := check("GetCurrentMediaType", reader.call(mGetCurrentMediaType, firstVideoStream, uintptr(unsafe.Pointer(&t)))); err != nil {
		return 0, 0, err
	}
	defer t.release()
	var size uint64
	if err := check("MF_MT_FRAME_SIZE", t.call(mGetUINT64, uintptr(unsafe.Pointer(&guidFrameSize)), uintptr(unsafe.Pointer(&size)))); err != nil {
		return 0, 0, err
	}
	w, h = int(size>>32), int(uint32(size))
	if w <= 0 || h <= 0 {
		return 0, 0, errors.New("webcam: no picture size")
	}
	return w, h, nil
}

// readSamples gives s the luma of each picture until stop is closed, or
// the camera stops.
func readSamples(reader *com, s *Stream, w, h int, stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		default:
		}
		var index, flags uint32
		var timestamp int64
		var sample *com
		r := reader.call(mReadSample, firstVideoStream, 0, uintptr(unsafe.Pointer(&index)),
			uintptr(unsafe.Pointer(&flags)), uintptr(unsafe.Pointer(&timestamp)), uintptr(unsafe.Pointer(&sample)))
		if int32(r) < 0 || flags&(readerfError|readerfEnd) != 0 {
			sample.release()
			return // unplugged
		}
		if sample == nil {
			continue // a gap in the stream
		}
		keepSample(sample, s, w, h)
		sample.release()
	}
}

// keepSample gives s the luma plane of an NV12 sample.
func keepSample(sample *com, s *Stream, w, h int) {
	var buf *com
	if int32(sample.call(mConvertToContiguous, uintptr(unsafe.Pointer(&buf)))) < 0 {
		return
	}
	defer buf.release()
	var data *byte
	var maxLen, length uint32
	if int32(buf.call(mLock, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&maxLen)), uintptr(unsafe.Pointer(&length)))) < 0 {
		return
	}
	defer buf.call(mUnlock)
	if data != nil && int(length) >= w*h {
		s.put(unsafe.Slice(data, w*h), w, h, w)
	}
}
