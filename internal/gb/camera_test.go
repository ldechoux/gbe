package gb

import (
	"archive/zip"
	"io"
	"path/filepath"
	"testing"
)

// cameraGB is a console with a Game Boy Camera cartridge whose ROM does
// nothing, the camera driven by hand.
func cameraGB(t *testing.T) (*GameBoy, *camera) {
	t.Helper()
	cart := newTestCart(t, 0xFC, 4, 64)
	g, err := NewModel(cart, nil, ModelDMG)
	if err != nil {
		t.Fatal(err)
	}
	return g, cart.mbc.(*camera)
}

// setCamera maps the registers, then sets the exposure to 0x1000 and the
// gain to 1, for a light of the picture as seen, and thresholds of 64, 128
// and 192 for every pixel of the matrix.
func setCamera(c *Cartridge, a001 byte) {
	c.writeROM(0x4000, 0x10)
	for addr, v := range map[uint16]byte{0xA001: a001, 0xA002: 0x10, 0xA003: 0x00, 0xA004: 0x20} {
		c.writeRAM(addr, v)
	}
	for i := range 16 {
		for j, v := range []byte{64, 128, 192} {
			c.writeRAM(0xA006+uint16(i*3+j), v)
		}
	}
}

// shadeAt is the shade of the pixel (x, y) of the picture in RAM bank 0.
func shadeAt(ram []byte, x, y int) int {
	i := cameraImage + (y/8*16+x/8)*16 + y%8*2
	bit := 7 - x%8
	return int(ram[i]>>bit&1) | int(ram[i+1]>>bit&1)<<1
}

func TestCameraBanks(t *testing.T) {
	c := newTestCart(t, 0xFC, 4, 64)
	if got := bank(c, 0x4000); got != 1 {
		t.Errorf("ROM bank %d at power on, want 1", got)
	}
	for _, b := range []byte{0, 0x2A, 0x3F} {
		c.writeROM(0x2000, b)
		if got := bank(c, 0x4000); got != int(b) {
			t.Errorf("ROM bank %d, want %d", got, b)
		}
	}

	// RAM writes need the RAM enabled, reads do not.
	c.writeROM(0x4000, 0x0F)
	c.writeRAM(0xA123, 0x55)
	if got := c.readRAM(0xA123); got != 0 {
		t.Errorf("a write with the RAM disabled reached it: %02X", got)
	}
	c.writeROM(0x0000, 0x0A)
	c.writeRAM(0xA123, 0x55)
	if got := c.ram[15*0x2000+0x123]; got != 0x55 {
		t.Errorf("RAM bank 15 holds %02X", got)
	}

	// Bank 0x10 maps the registers, mirrored every 0x80 bytes: only A000
	// reads back, its 3 low bits.
	c.writeROM(0x4000, 0x10)
	c.writeRAM(0xA081, 0xE4)
	c.writeRAM(0xA000, 0xF6)
	m := c.mbc.(*camera)
	if m.regs[1] != 0xE4 || c.readRAM(0xA000) != 0x06 || c.readRAM(0xA001) != 0 {
		t.Errorf("registers %02X, A000 reads %02X, A001 %02X", m.regs[:2], c.readRAM(0xA000), c.readRAM(0xA001))
	}
	if c.ram[15*0x2000+0x123] != 0x55 || c.ram[0x81] != 0 {
		t.Error("a register write reached the RAM")
	}
}

// A capture takes the time Pan Docs gives, the RAM held meanwhile, then
// writes the picture to RAM bank 0 in tiles, each pixel shaded by the
// thresholds of the matrix.
func TestCameraCapture(t *testing.T) {
	g, m := cameraGB(t)
	c := g.Cart
	c.writeROM(0x0000, 0x0A)
	c.ram[cameraImage] = 0x99
	setCamera(c, 0x04) // N clear: 512 more M-cycles
	c.SetCameraSource(func(img *CameraImage) bool {
		for i := range img {
			img[i] = byte(i % CameraWidth * 2) // dark at the left
		}
		return true
	})

	c.writeRAM(0xA000, 0x01)
	want := uint64(4 * (32446 + 512 + 16*0x1000))
	if m.done != g.Bus.cycles+want {
		t.Errorf("the capture takes %d T-cycles, want %d", m.done-g.Bus.cycles, want)
	}
	g.Bus.cycles += want - 1
	c.writeROM(0x4000, 0x00)
	if c.readRAM(0xA100) != 0 {
		t.Error("the RAM can be read during a capture")
	}
	c.writeRAM(0xA100, 0x12)
	c.writeROM(0x4000, 0x10)
	if c.readRAM(0xA000)&1 != 1 {
		t.Fatal("the capture ended early")
	}
	g.Bus.cycles++
	if c.readRAM(0xA000)&1 != 0 {
		t.Fatal("the capture did not end")
	}
	for _, p := range []struct{ x, shade int }{{0, 3}, {31, 3}, {32, 2}, {63, 2}, {64, 1}, {95, 1}, {96, 0}, {127, 0}} {
		for _, y := range []int{0, 57, 111} {
			if got := shadeAt(c.ram, p.x, y); got != p.shade {
				t.Errorf("pixel (%d, %d): shade %d, want %d", p.x, y, got, p.shade)
			}
		}
	}
	if !c.Dirty() {
		t.Error("the picture is not saved")
	}

	// Double the exposure: the light doubles, a pixel at 40 reads as 80.
	c.writeRAM(0xA002, 0x20)
	c.writeRAM(0xA000, 0x01)
	g.Bus.cycles = m.done
	c.readRAM(0xA000)
	if got := shadeAt(c.ram, 20, 0); got != 2 {
		t.Errorf("pixel 20 at twice the exposure: shade %d, want 2", got)
	}
}

// Without a picture, or with a source that has none, the sensor sees
// noise: the same for the same capture.
func TestCameraNoise(t *testing.T) {
	var a, b CameraImage
	cameraNoise(&a, 1234)
	cameraNoise(&b, 1234)
	if a != b {
		t.Error("the same capture gave two pictures")
	}
	cameraNoise(&b, 1235)
	if a == b {
		t.Error("two captures gave the same noise")
	}
	var seen [4]bool
	for _, v := range a {
		seen[v>>6] = true
	}
	if seen != [4]bool{true, true, true, true} {
		t.Errorf("the noise misses levels: %v", seen)
	}

	g, m := cameraGB(t)
	g.Cart.SetCameraSource(func(*CameraImage) bool { return false })
	setCamera(g.Cart, 0x04)
	g.Cart.writeRAM(0xA000, 0x01)
	g.Bus.cycles = m.done
	g.Cart.readRAM(0xA000)
	var shades [4]int
	for y := range CameraHeight {
		for x := range CameraWidth {
			shades[shadeAt(g.Cart.ram, x, y)]++
		}
	}
	for s, n := range shades {
		if n < 1000 {
			t.Errorf("shade %d: %d pixels of noise", s, n)
		}
	}
}

// In the 2D mode of the ROM (A001 bits 7-5 set), the edges are enhanced:
// each side of a step moves away from the other.
func TestCameraEdges(t *testing.T) {
	for _, c := range []struct {
		a001        byte
		left, right int
	}{
		{0x04, 2, 1}, // 100 and 150, as seen
		{0xE4, 3, 0}, // 50 and 200 at the step, with a ratio of 1
	} {
		g, m := cameraGB(t)
		setCamera(g.Cart, c.a001)
		g.Cart.SetCameraSource(func(img *CameraImage) bool {
			for i := range img {
				img[i] = 100
				if i%CameraWidth >= 64 {
					img[i] = 150
				}
			}
			return true
		})
		g.Cart.writeRAM(0xA000, 0x01)
		g.Bus.cycles = m.done
		g.Cart.readRAM(0xA000)
		if l, r := shadeAt(g.Cart.ram, 63, 50), shadeAt(g.Cart.ram, 64, 50); l != c.left || r != c.right {
			t.Errorf("A001 %02X: shades %d and %d at the step, want %d and %d", c.a001, l, r, c.left, c.right)
		}
	}
}

// A state saved during a capture ends it after loading, at the same time;
// a reset drops it.
func TestCameraState(t *testing.T) {
	g, m := cameraGB(t)
	setCamera(g.Cart, 0x84)
	g.Cart.writeRAM(0xA000, 0x01)
	done := m.done
	snap := g.Snapshot(nil)
	state := g.SaveState()

	g.Bus.cycles = done
	if g.Cart.readRAM(0xA000)&1 != 0 {
		t.Fatal("the capture did not end")
	}
	if err := g.Restore(snap); err != nil {
		t.Fatal(err)
	}
	if !m.busy || m.done != done || m.regs[1] != 0x84 {
		t.Errorf("after Restore: busy %v, end %d (want %d), A001 %02X", m.busy, m.done, done, m.regs[1])
	}

	g2, m2 := cameraGB(t)
	if err := g2.LoadState(state); err != nil {
		t.Fatal(err)
	}
	if !m2.busy || m2.done != done {
		t.Errorf("after LoadState: busy %v, end %d, want %d", m2.busy, m2.done, done)
	}
	g2.Reset()
	if m2.busy || g2.Cart.readRAM(0xA000)&1 != 0 {
		t.Error("the capture survived a reset")
	}
}

// The camera ROM, through its menus to the viewfinder: its automatic
// exposure settles, and the screen shows the picture the sensor sees.
// Skipped without the ROM (roms/).
func TestGameBoyCameraROM(t *testing.T) {
	zr, err := zip.OpenReader(filepath.Join(gameDir, "Game Boy Camera (USA, Europe) (SGB Enhanced).zip"))
	if err != nil {
		t.Skip(err)
	}
	defer zr.Close()
	f, err := zr.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	rom, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	cart, err := NewCartridge(rom)
	if err != nil {
		t.Fatal(err)
	}
	if !cart.IsCamera() {
		t.Fatal("not a camera")
	}
	g, err := NewModel(cart, nil, ModelDMG)
	if err != nil {
		t.Fatal(err)
	}
	// Black at the left, white at the right.
	cart.SetCameraSource(func(img *CameraImage) bool {
		for i := range img {
			img[i] = byte(i % CameraWidth * 2)
		}
		return true
	})
	captures := 0
	m := cart.mbc.(*camera)
	for frame := range 2000 {
		// The title screen, the main menu, then Shoot twice.
		pressed := false
		for _, at := range []int{300, 400, 500, 600, 700, 1300, 1450, 1600} {
			pressed = pressed || frame >= at && frame < at+5
		}
		g.SetButton(ButtonA, pressed && frame > 300)
		g.SetButton(ButtonStart, frame >= 300 && frame < 305)
		busy := m.busy
		g.RunFrame()
		g.APU.DrainSamples()
		if m.busy && !busy {
			captures++
		}
	}
	if captures < 50 {
		t.Fatalf("%d captures, want the viewfinder", captures)
	}
	// The viewfinder fills the screen from (16, 16): a dark left, a light
	// right.
	fb := g.Framebuffer()
	var left, right int
	for y := 40; y < 100; y++ {
		for x := 24; x < 56; x++ {
			left += int(fb[y*ScreenWidth+x] & 3)
			right += int(fb[y*ScreenWidth+x+80] & 3)
		}
	}
	if left <= right*2 {
		t.Errorf("shades: %d at the left, %d at the right, want the left darker", left, right)
	}
	t.Logf("%d captures, exposure %#04x", captures, m.exposure())
}
