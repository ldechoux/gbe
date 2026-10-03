# ROM de benchmark

`gbe-bench.gbc` est une ROM Game Boy écrite pour les benchmarks et les tests de gbe.
Contrairement aux jeux et aux ROMs de test, elle est libre de droits. Elle est donc dans le
dépôt et tourne partout, la CI comprise. Elle fonctionne sur DMG et sur Game Boy Color.

Chaque scène reproduit la charge d'un type de jeu. Le bouton tenu à l'allumage choisit la
scène :

| Bouton | Scène | Ce qu'elle fait | Comme |
|---|---|---|---|
| A | game | Défilement, HUD dans la fenêtre, découpe de l'écran (raster), 40 sprites (dont 12 sur les mêmes lignes), musique sur les 4 canaux | Zelda |
| B | cpu | Calcul sans jamais de pause : CRC sur des banques de ROM commutées (MBC5), multiplications, divisions, copies entre banques de WRAM, RAM de cartouche | cpu_instrs |
| Select | idle | Presque toujours en pause : un sprite, un bip de temps en temps | Super Mario Land |
| Start | color | Double vitesse CGB, DMA HBlank, changement de palette à chaque ligne, attributs et retournements des tuiles. Sur DMG, BGP change toutes les 16 lignes | Zelda DX |
| Droite | sound | Les 4 canaux redéclenchés avec des réglages aléatoires : sweep, bruit court et long, nouvelle wave RAM, panoramique | |
| aucun | demo | Toutes les scènes à tour de rôle, 256 frames chacune | |

Tout est déterministe : mêmes images et mêmes échantillons à chaque exécution.

## Utilisation dans gbe

- **Tests** (`internal/gb/benchrom_test.go`) :
  - chaque scène tourne en DMG et en CGB ;
  - l'empreinte de chaque exécution (son, chaque image, état final) ne doit pas changer ;
  - la dernière image doit correspondre à son image de référence dans `ref/`.

  En cas d'échec, l'image et le son obtenus sont écrits dans `$GBE_GOLDEN_OUT` s'il est
  défini. Après un changement voulu : `go test ./internal/gb -run BenchROM -update`.
- **Benchmarks** : `BenchmarkFrameBenchROM/<scène>/<dmg|cgb>`, aussi lancés par la CI sur
  chaque pull request pour la comparer à sa branche de base.

## Compilation

Les sources sont dans `src/`, pour [RGBDS](https://rgbds.gbdev.io) 1.0 :

```sh
make -C benchrom   # rgbasm, rgblink et rgbfix dans le PATH
```

La CI recompile la ROM et vérifie qu'elle est identique à celle du dépôt.

## Vérification avec un autre émulateur

Les images de référence viennent de gbe. Pour s'assurer qu'elles montrent ce que fait le
matériel, la ROM a été comparée avec `sameboy_tester` de [SameBoy](https://sameboy.github.io)
(`make tester` dans ses sources).

Deux options de compilation servent à cela :
- `FORCE_SCENE=n` choisit la scène, car le testeur ne peut pas tenir de bouton.
- `FREEZE_AT=frame` fige la scène à cette frame. L'image ne dépend alors plus de la durée de
  la boot ROM de chaque émulateur.

```sh
rgbasm -I src/ -D FORCE_SCENE=1 -D FREEZE_AT=200 -o scene.o src/main.asm
rgblink -o scene.gbc scene.o
rgbfix -v -c -m 0x1A -r 2 -t "GBE BENCH" -p 0xFF scene.gbc
sameboy_tester --cgb --length 12 scene.gbc               # écrit scene.bmp
gbe -bios none -model gbc -frames 700 -screenshot scene.png scene.gbc
```

Figées à la frame 200, les dix images (5 scènes × DMG/CGB) sont identiques au pixel près
dans les deux émulateurs, une fois leurs palettes de couleurs mises en correspondance.

Figée à la frame 500, la scène son en CGB affiche une autre valeur de `NR52` : SameBoy fait
durer le canal 3 plus longtemps. C'est l'indice d'un écart de l'APU de gbe en mode Game Boy
Color, qui reste à étudier. Il n'affecte pas les tests, dont les références viennent de gbe.

Les effets raster écrivent leurs registres pendant le HBlank, comme le font les jeux. Une
première version les changeait au milieu d'une ligne, un timing que gbe n'émule pas.
