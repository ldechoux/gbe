# Benchmark ROM

`gbe-bench.gbc` is a Game Boy ROM written for gbe's benchmarks and tests. Unlike
the games and the test ROMs, it is free to distribute, so it is committed and runs
everywhere, the CI included. It runs on a DMG and on a Game Boy Color.

Each scene puts the emulator under the load of a kind of game. The scene is chosen
by the button held at power on:

| Button | Scene | What it does | Like |
|---|---|---|---|
| A | game | Scrolling, a HUD in the window, a raster split, 40 sprites (12 on the same lines), music on the four channels | Zelda |
| B | cpu | Computations that never halt: CRC over switched ROM banks (MBC5), multiplications, divisions, copies between WRAM banks, cartridge RAM | cpu_instrs |
| Select | idle | Halted most of the time: one sprite, a beep now and then | Super Mario Land |
| Start | color | CGB double speed, HBlank DMA, a palette change on every line, tile attributes and flips. On a DMG, BGP changes every 16 lines | Zelda DX |
| Right | sound | The four channels retriggered with random settings: sweep, both noise widths, new wave RAM, panning | |
| none | demo | Every scene in turn, 256 frames each | |

Everything is deterministic: the same frames and samples on every run.

## Use in gbe

- **Tests** (`internal/gb/benchrom_test.go`):
  - each scene runs on a DMG and on a CGB;
  - the golden hash of each run (audio, every frame, final state) must not change;
  - the last frame must match its reference image in `ref/`.

  On failure, the frame and the audio are written to `$GBE_GOLDEN_OUT` if it is set.
  After an intended change: `go test ./internal/gb -run BenchROM -update`.
- **Benchmarks:** `BenchmarkFrameBenchROM/<scene>/<dmg|cgb>`, also run by the CI on
  every pull request to compare it with its base branch.

## Building

The sources are in `src/`, for [RGBDS](https://rgbds.gbdev.io) 1.0:

```sh
make -C benchrom   # needs rgbasm, rgblink and rgbfix in the PATH
```

The CI rebuilds the ROM and checks that it matches the committed one.

## Checking it against another emulator

The reference images come from gbe. To make sure they show what the hardware does,
the ROM was compared with [SameBoy](https://sameboy.github.io)'s `sameboy_tester`
(`make tester` in its sources).

Two build options exist for that:
- `FORCE_SCENE=n` selects the scene, since the tester cannot hold a button.
- `FREEZE_AT=frame` stops the scene at that frame, so that the picture does not
  depend on how long each emulator's boot ROM took.

```sh
rgbasm -I src/ -D FORCE_SCENE=1 -D FREEZE_AT=200 -o scene.o src/main.asm
rgblink -o scene.gbc scene.o
rgbfix -v -c -m 0x1A -r 2 -t "GBE BENCH" -p 0xFF scene.gbc
sameboy_tester --cgb --length 12 scene.gbc               # writes scene.bmp
gbe -bios none -model gbc -frames 700 -screenshot scene.png scene.gbc
```

Frozen at frame 200, all ten pictures (5 scenes × DMG/CGB) are the same in both
emulators, pixel for pixel (once their color palettes are matched).

Frozen at frame 500, the CGB sound scene shows another `NR52` value: SameBoy keeps
channel 3 playing longer. This points to a difference in gbe's APU on a Game Boy
Color, still to investigate. It does not affect the tests, whose references come
from gbe.

The raster effects write their registers during HBlank, as games do. Earlier
versions changed them in the middle of a line, a timing gbe does not emulate.
