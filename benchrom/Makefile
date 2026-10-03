# Builds the benchmark ROM with RGBDS (https://rgbds.gbdev.io).
# The ROM is committed, so this is only needed after changing the sources;
# the CI checks that the committed ROM matches them.

RGBASM  ?= rgbasm
RGBLINK ?= rgblink
RGBFIX  ?= rgbfix

ROM  := gbe-bench.gbc
SRCS := $(wildcard src/*.asm src/*.inc)

$(ROM): $(SRCS)
	$(RGBASM) -Wall -I src/ -o main.o src/main.asm
	$(RGBLINK) -n gbe-bench.sym -m gbe-bench.map -o $@ main.o
	# CGB compatible, MBC5 with 8 KiB of RAM and no battery
	$(RGBFIX) -v -c -m 0x1A -r 2 -t "GBE BENCH" -p 0xFF $@
	rm -f main.o

clean:
	rm -f $(ROM) main.o gbe-bench.sym gbe-bench.map

.PHONY: clean
