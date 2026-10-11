package webcam

// macOS: AVFoundation, through the Objective-C runtime (purego/objc). A
// capture session sends the pictures of the camera, in the 4:2:0
// bi-planar format whose first plane is the luma, to a delegate class
// registered at run time, on a dispatch queue.

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

const (
	pixelFormat420f = 0x34323066 // kCVPixelFormatType_420YpCbCr8BiPlanarFullRange, '420f'
	lockReadOnly    = 1          // kCVPixelBufferLock_ReadOnly

	authNotDetermined = 0 // AVAuthorizationStatus
	authAuthorized    = 3
)

var (
	loadOnce sync.Once
	loadErr  error

	mediaTypeVideo objc.ID // AVMediaTypeVideo
	formatTypeKey  objc.ID // kCVPixelBufferPixelFormatTypeKey
	preset640x480  objc.ID // AVCaptureSessionPreset640x480

	dispatchQueueCreate func(label string, attr uintptr) uintptr
	dispatchRelease     func(obj uintptr)

	sampleBufferGetImageBuffer func(sb uintptr) uintptr
	pixelBufferLock            func(pb uintptr, flags uint64) int32
	pixelBufferUnlock          func(pb uintptr, flags uint64) int32
	pixelBufferPlaneAddress    func(pb uintptr, plane uintptr) uintptr
	pixelBufferPlaneWidth      func(pb uintptr, plane uintptr) uintptr
	pixelBufferPlaneHeight     func(pb uintptr, plane uintptr) uintptr
	pixelBufferPlaneStride     func(pb uintptr, plane uintptr) uintptr
	pixelBufferFormat          func(pb uintptr) uint32

	delegateClass objc.Class

	// streams are the open streams, by their delegate: the delegate
	// method finds its stream there.
	streamsMu sync.Mutex
	streams   = map[objc.ID]*Stream{}
)

var (
	selAlloc        = objc.RegisterName("alloc")
	selInit         = objc.RegisterName("init")
	selRelease      = objc.RegisterName("release")
	selCount        = objc.RegisterName("count")
	selObjectAt     = objc.RegisterName("objectAtIndex:")
	selUTF8String   = objc.RegisterName("UTF8String")
	selStringWith   = objc.RegisterName("stringWithUTF8String:")
	selUniqueID     = objc.RegisterName("uniqueID")
	selLocalized    = objc.RegisterName("localizedName")
	selDevicesWith  = objc.RegisterName("devicesWithMediaType:")
	selDefaultWith  = objc.RegisterName("defaultDeviceWithMediaType:")
	selDeviceWithID = objc.RegisterName("deviceWithUniqueID:")
	selAuthStatus   = objc.RegisterName("authorizationStatusForMediaType:")
	selRequest      = objc.RegisterName("requestAccessForMediaType:completionHandler:")
	selInputWith    = objc.RegisterName("deviceInputWithDevice:error:")
	selCanAddInput  = objc.RegisterName("canAddInput:")
	selAddInput     = objc.RegisterName("addInput:")
	selCanAddOutput = objc.RegisterName("canAddOutput:")
	selAddOutput    = objc.RegisterName("addOutput:")
	selCanPreset    = objc.RegisterName("canSetSessionPreset:")
	selSetPreset    = objc.RegisterName("setSessionPreset:")
	selSetSettings  = objc.RegisterName("setVideoSettings:")
	selSetDiscard   = objc.RegisterName("setAlwaysDiscardsLateVideoFrames:")
	selSetDelegate  = objc.RegisterName("setSampleBufferDelegate:queue:")
	selStart        = objc.RegisterName("startRunning")
	selStop         = objc.RegisterName("stopRunning")
	selNumberWith   = objc.RegisterName("numberWithUnsignedInt:")
	selDictWith     = objc.RegisterName("dictionaryWithObject:forKey:")
	selDescription  = objc.RegisterName("localizedDescription")
	selDidOutput    = objc.RegisterName("captureOutput:didOutputSampleBuffer:fromConnection:")
)

// load opens the frameworks, reads their constants and registers the
// delegate class, once.
func load() error {
	loadOnce.Do(func() { loadErr = loadFrameworks() })
	return loadErr
}

func loadFrameworks() error {
	open := func(path string) (uintptr, error) {
		lib, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return 0, fmt.Errorf("webcam: %w", err)
		}
		return lib, nil
	}
	av, err := open("/System/Library/Frameworks/AVFoundation.framework/AVFoundation")
	if err != nil {
		return err
	}
	cm, err := open("/System/Library/Frameworks/CoreMedia.framework/CoreMedia")
	if err != nil {
		return err
	}
	cv, err := open("/System/Library/Frameworks/CoreVideo.framework/CoreVideo")
	if err != nil {
		return err
	}
	system, err := open("/usr/lib/libSystem.B.dylib")
	if err != nil {
		return err
	}
	constant := func(lib uintptr, name string) (objc.ID, error) {
		sym, err := purego.Dlsym(lib, name)
		if err != nil {
			return 0, fmt.Errorf("webcam: %w", err)
		}
		return **(**objc.ID)(unsafe.Pointer(&sym)), nil
	}
	if mediaTypeVideo, err = constant(av, "AVMediaTypeVideo"); err != nil {
		return err
	}
	if preset640x480, err = constant(av, "AVCaptureSessionPreset640x480"); err != nil {
		return err
	}
	if formatTypeKey, err = constant(cv, "kCVPixelBufferPixelFormatTypeKey"); err != nil {
		return err
	}
	purego.RegisterLibFunc(&dispatchQueueCreate, system, "dispatch_queue_create")
	purego.RegisterLibFunc(&dispatchRelease, system, "dispatch_release")
	purego.RegisterLibFunc(&sampleBufferGetImageBuffer, cm, "CMSampleBufferGetImageBuffer")
	purego.RegisterLibFunc(&pixelBufferLock, cv, "CVPixelBufferLockBaseAddress")
	purego.RegisterLibFunc(&pixelBufferUnlock, cv, "CVPixelBufferUnlockBaseAddress")
	purego.RegisterLibFunc(&pixelBufferPlaneAddress, cv, "CVPixelBufferGetBaseAddressOfPlane")
	purego.RegisterLibFunc(&pixelBufferPlaneWidth, cv, "CVPixelBufferGetWidthOfPlane")
	purego.RegisterLibFunc(&pixelBufferPlaneHeight, cv, "CVPixelBufferGetHeightOfPlane")
	purego.RegisterLibFunc(&pixelBufferPlaneStride, cv, "CVPixelBufferGetBytesPerRowOfPlane")
	purego.RegisterLibFunc(&pixelBufferFormat, cv, "CVPixelBufferGetPixelFormatType")

	var protocols []*objc.Protocol
	if p := objc.GetProtocol("AVCaptureVideoDataOutputSampleBufferDelegate"); p != nil {
		protocols = append(protocols, p)
	}
	delegateClass, err = objc.RegisterClass("GBEWebcamDelegate", objc.GetClass("NSObject"), protocols, nil,
		[]objc.MethodDef{{Cmd: selDidOutput, Fn: didOutput}})
	return err
}

// didOutput is the method of the delegate that receives each picture, on
// the queue of the session: it keeps the luma plane.
func didOutput(self objc.ID, _ objc.SEL, _, sampleBuffer, _ objc.ID) {
	streamsMu.Lock()
	s := streams[self]
	streamsMu.Unlock()
	if s == nil {
		return
	}
	pb := sampleBufferGetImageBuffer(uintptr(sampleBuffer))
	if pb == 0 || pixelBufferFormat(pb) != pixelFormat420f || pixelBufferLock(pb, lockReadOnly) != 0 {
		return
	}
	defer pixelBufferUnlock(pb, lockReadOnly)
	base := pixelBufferPlaneAddress(pb, 0)
	w, h := int(pixelBufferPlaneWidth(pb, 0)), int(pixelBufferPlaneHeight(pb, 0))
	stride := int(pixelBufferPlaneStride(pb, 0))
	if base == 0 || w <= 0 || h <= 0 || stride < w {
		return
	}
	s.put(unsafe.Slice(*(**byte)(unsafe.Pointer(&base)), stride*(h-1)+w), w, h, stride)
}

func nsString(s string) objc.ID {
	return objc.ID(objc.GetClass("NSString")).Send(selStringWith, s)
}

func goString(str objc.ID) string {
	if str == 0 {
		return ""
	}
	return objc.Send[string](str, selUTF8String)
}

// Devices lists the cameras of the system.
func Devices() ([]Device, error) {
	if err := load(); err != nil {
		return nil, err
	}
	list := objc.ID(objc.GetClass("AVCaptureDevice")).Send(selDevicesWith, mediaTypeVideo)
	n := objc.Send[uint](list, selCount)
	devices := make([]Device, 0, n)
	for i := range n {
		d := list.Send(selObjectAt, i)
		devices = append(devices, Device{ID: goString(d.Send(selUniqueID)), Name: goString(d.Send(selLocalized))})
	}
	return devices, nil
}

// authorize asks the player for the camera the first time; it blocks until
// they answer.
func authorize() error {
	class := objc.ID(objc.GetClass("AVCaptureDevice"))
	switch objc.Send[int](class, selAuthStatus, mediaTypeVideo) {
	case authAuthorized:
		return nil
	case authNotDetermined:
	default:
		return ErrDenied
	}
	answer := make(chan bool, 1)
	block := objc.NewBlock(func(_ objc.Block, granted bool) { answer <- granted })
	defer block.Release()
	class.Send(selRequest, mediaTypeVideo, block)
	if !<-answer {
		return ErrDenied
	}
	return nil
}

// Open starts the camera id ("" for the default one). The first time, macOS
// asks the player whether gbe may use the camera: Open waits for the
// answer.
func Open(id string) (*Stream, error) {
	if err := load(); err != nil {
		return nil, err
	}
	if err := authorize(); err != nil {
		return nil, err
	}
	class := objc.ID(objc.GetClass("AVCaptureDevice"))
	var device objc.ID
	if id != "" {
		device = class.Send(selDeviceWithID, nsString(id))
	} else {
		device = class.Send(selDefaultWith, mediaTypeVideo)
	}
	if device == 0 {
		return nil, ErrNoDevice
	}
	var nsErr objc.ID
	input := objc.ID(objc.GetClass("AVCaptureDeviceInput")).Send(selInputWith, device, unsafe.Pointer(&nsErr))
	if input == 0 {
		return nil, fmt.Errorf("webcam: %s", goString(nsErr.Send(selDescription)))
	}

	session := objc.ID(objc.GetClass("AVCaptureSession")).Send(selAlloc).Send(selInit)
	output := objc.ID(objc.GetClass("AVCaptureVideoDataOutput")).Send(selAlloc).Send(selInit)
	if objc.Send[bool](session, selCanPreset, preset640x480) {
		session.Send(selSetPreset, preset640x480)
	}
	if !objc.Send[bool](session, selCanAddInput, input) || !objc.Send[bool](session, selCanAddOutput, output) {
		output.Send(selRelease)
		session.Send(selRelease)
		return nil, fmt.Errorf("webcam: the camera cannot be used")
	}
	session.Send(selAddInput, input)
	format := objc.ID(objc.GetClass("NSNumber")).Send(selNumberWith, uint32(pixelFormat420f))
	output.Send(selSetSettings, objc.ID(objc.GetClass("NSDictionary")).Send(selDictWith, format, formatTypeKey))
	output.Send(selSetDiscard, true)

	s := &Stream{}
	delegate := objc.ID(delegateClass).Send(selAlloc).Send(selInit)
	streamsMu.Lock()
	streams[delegate] = s
	streamsMu.Unlock()
	queue := dispatchQueueCreate("gbe.webcam", 0)
	output.Send(selSetDelegate, delegate, queue)
	session.Send(selAddOutput, output)
	session.Send(selStart)

	s.closer = func() {
		session.Send(selStop) // waits for the delegate to be done
		output.Send(selSetDelegate, objc.ID(0), uintptr(0))
		streamsMu.Lock()
		delete(streams, delegate)
		streamsMu.Unlock()
		output.Send(selRelease)
		session.Send(selRelease)
		delegate.Send(selRelease)
		dispatchRelease(queue)
	}
	return s, nil
}
