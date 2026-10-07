// Package icon holds the icon of the application, drawn by tools/icongen:
// PNG files at several sizes, gbe.icns for macOS and gbe.ico for Windows.
package icon

//go:generate go run ../../tools/icongen -out .

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/png"
)

//go:embed icon-16.png icon-32.png icon-48.png
var files embed.FS

// Window returns the icon at the sizes a window title bar and a task bar
// use. macOS ignores it: it takes the icon of the application bundle.
func Window() []image.Image {
	var imgs []image.Image
	for _, n := range []int{16, 32, 48} {
		b, err := files.ReadFile(fmt.Sprintf("icon-%d.png", n))
		if err != nil {
			panic(err)
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			panic(err)
		}
		imgs = append(imgs, img)
	}
	return imgs
}
