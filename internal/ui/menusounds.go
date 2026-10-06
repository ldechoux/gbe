package ui

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2/audio"

	"github.com/ldechoux/gbe/internal/menusound"
)

// soundPlayer plays the sounds of the menus.
type soundPlayer interface {
	play(k menusound.Kind, volume float64)
}

// ebitenSounds plays each menu sound with its own player, beside the one of
// the game, which stays frozen while the menu is open. A sound played again
// starts over.
type ebitenSounds [menusound.Refuse + 1]*audio.Player

func newEbitenSounds(ctx *audio.Context) *ebitenSounds {
	var s ebitenSounds
	for k := range s {
		s[k] = ctx.NewPlayerFromBytes(menusound.PCM(menusound.Kind(k)))
	}
	return &s
}

func (s *ebitenSounds) play(k menusound.Kind, volume float64) {
	p := s[k]
	p.SetVolume(volume)
	if err := p.Rewind(); err != nil {
		log.Printf("menu sound: %v", err)
		return
	}
	p.Play()
}

// menuSound plays the sound of k, at the volume of the settings, unless the
// menu sounds are off.
func (g *Game) menuSound(k menusound.Kind) {
	if g.sfx != nil && g.cfg.MenuSounds {
		g.sfx.play(k, g.cfg.Volume)
	}
}
