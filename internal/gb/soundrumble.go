package gb

// SoundRumble guesses from the sound being played how hard a game without a
// rumble motor would shake, from 0 to 1, as SameBoy does. Games give shocks
// typical sounds: loud low noise on channel 4 for explosions and impacts,
// and a fast loud sweep on channel 1 for shots and falls. The melody, on the
// other channels, never shakes. It only reads the APU, as it is at the end
// of the frame.
func (g *GameBoy) SoundRumble() float64 {
	a := g.APU
	if !a.on {
		return 0
	}
	nr50, nr51 := a.regs[0x14], a.regs[0x15]
	master := float64(int(nr50&7) + 1 + int(nr50>>4&7) + 1) // both sides, 2 to 16
	// The sides channel ch is sent to: 0, 1 or 2.
	sides := func(ch byte) float64 {
		return float64(nr51>>ch&1 + nr51>>(ch+4)&1)
	}
	return min(max(noiseRumble(a, master*sides(3))+sweepRumble(a, master*sides(0))/2, 0), 1)
}

// noiseRumble is how hard channel 4 shakes, at volume (master volume times
// sides): the lower and louder the noise, the harder.
func noiseRumble(a *APU, volume float64) float64 {
	n := &a.ch[3]
	if !n.enabled {
		return 0
	}
	nr43 := a.regs[0x12]
	divisor := int(nr43&7) << 1
	if divisor == 0 {
		divisor = 1
	}
	period := divisor<<(nr43>>4) - 1 // of the LFSR clock: low noises are long
	if n.narrow {
		period *= 8
	}
	env := float64(n.env.volume)
	r := (float64(min(period, 4096))*(env*env*volume/32-50) - 2048) / 2048
	return min(max(r, 0), 1)
}

// sweepRumble is how hard channel 1 shakes, at volume (master volume times
// sides): only while its frequency sweeps, the faster and louder, the harder.
func sweepRumble(a *APU, volume float64) float64 {
	c := &a.ch[0]
	nr10 := a.regs[0x00]
	shift, pace := nr10&7, nr10>>4&7
	if !c.enabled || shift == 0 || pace == 0 {
		return 0
	}
	speed := float64(shift) / float64(pace)
	r := float64(c.env.volume)*volume/32*speed/8 - 0.5
	return min(max(r, 0), 1)
}
