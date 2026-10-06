---
name: site-captures
description: Refait les captures du site du projet (toutes les images de screenshots/gb et screenshots/gbc) avec le rendu de gbe — l'en-tête, les palettes, les jeux DMG colorisés, les détails des filtres d'affichage, les captures ×8 au filtre LCD de la section « Captures » et l'écran de dépôt des ROMs — puis prépare l'aperçu du site. À utiliser quand l'utilisateur demande de refaire, mettre à jour ou régénérer les captures ou les images du site, par exemple après un changement du menu, d'un filtre, d'une palette, de la colorisation ou de l'écran de dépôt.
---

# Refaire les captures du site

Toutes les images du site (`site/index.html.tmpl`) viennent de `screenshots/`, qui
`tools/sitegen` copie dans `img/`. Le script de ce skill les refait toutes, par groupes, avec
le rendu de gbe, et des recettes fixes : même ROM, même image, mêmes réglages, d'une fois à
l'autre.

| Groupe | Fichiers | Scène | Section du site |
|---|---|---|---|
| `hero` | `gb/screenshot`, `gbc/screenshot` (640×576) | Les écrans titres de Super Mario Land (palette DMG vert) et de Link's Awakening DX (couleurs corrigées), sans filtre | En-tête (la console) |
| `palettes` | `gb/palette_{dmg,bgb,sepia,violet}` (640×576) | Le début du niveau 1-1 de Super Mario Land, dans les palettes DMG vert, BGB, Sepia et Violet | Game Boy : 10 palettes |
| `colorized` | `gbc/colorized_{tetris,zelda,sml,tetris_leftb}` (640×576) | Jeux DMG colorisés par la Game Boy Color : Tetris, Link's Awakening, Super Mario Land avec la palette choisie par la console, et Tetris avec la combinaison Gauche + B | Les jeux Game Boy en couleurs |
| `filters` | `{gb,gbc}/filter_lcd` (480×360), `gb/filter_{nearest,scale2x,scale3x,mmpx}` (480×432) | Des détails en taille réelle : le filtre LCD en ×15 (hauteur d'un écran 4K), les autres en ×8 | Grand écran |
| `scenes` | `{gb,gbc}/{game_title,menu,save_state_resume}` (1280×1152) | L'écran titre, le menu Pause par-dessus (en français, curseur sur Reprendre), l'écran de reprise au lancement ; en ×8 avec le filtre LCD | Captures (`#captures`) |
| `drop` | `gb/drop_screen` (1280×1152) | L'écran de dépôt des ROMs, gbe lancé sans ROM, avec 5 jeux récents | Télécharger, « Démarrage rapide » (`figure.drop-shot`) |

Les fichiers sont des WebP sans perte : on peut comparer une nouvelle capture à l'ancienne
pixel par pixel (`magick compare -metric AE`).

## Les recettes

Toutes tournent sans boot ROM. Les appuis sont tenus pendant 10 images, de l'image indiquée.

- **`hero`** : Super Mario Land en DMG, image 200 ; Link's Awakening DX en Game Boy Color,
  Start à l'image 500 pour passer l'intro, image 900. En ×4 par `ui.Screenshot`.
- **`palettes`** : Super Mario Land en DMG, Start à l'image 200, image 360 : Mario au début
  du niveau, près du tuyau. En ×4, avec les palettes `dmg`, `bgb`, `sepia` et `purple`
  (fichier `palette_violet`).
- **`colorized`** : en Game Boy Color, couleurs corrigées, ×4 :

  | Fichier | ROM | Start | Image | Palette |
  |---|---|---|---|---|
  | `colorized_tetris` | `Tetris_World_Rev1.gb` | 400 | 1300 | automatique |
  | `colorized_zelda` | `Legend_of_Zelda_The_Links_Awakening.gb` | 700 | 1500 | automatique |
  | `colorized_sml` | `Super_Mario_Land_World_Rev1.gb` | 200 | 400 | automatique |
  | `colorized_tetris_leftb` | `Tetris_World_Rev1.gb` | 400 | 1300 | `-compat 9` |

  « Automatique » est la palette que la console choisit d'après le titre (`gb.CompatAuto`).
  `-compat 9` est un index de `compatKeyCombos` (`internal/gb/compat_palettes.go`), dans
  l'ordre Droite, Gauche, Haut, Bas, puis les mêmes + A, puis + B : 9 est Gauche + B.
- **`filters`** :
  - les sources, en ×1 : Super Mario Land en DMG, Start à l'image 200, Droite tenue des
    images 300 à 360, image 360 ; Link's Awakening DX en Game Boy Color, couleurs
    corrigées, image 400 (le bateau de l'intro) ;
  - le rendu, par le vrai pipeline des filtres (`scaler.Pipeline`), pour chaque filtre :
    2400×2160 (×15) pour les deux sources, et 1280×1152 (×8) pour Super Mario Land. La
    couleur des interstices de la grille LCD DMG est le vert DMG `9BBC0F` ;
  - les détails : `gb/filter_lcd` = `lcd` ×15 de Super Mario Land, `480x360+0+0` (le
    « MARI » de la barre d'état) ; `gbc/filter_lcd` = `lcd` ×15 de Link's Awakening DX,
    `480x360+1890+1050` (le bateau sur les vagues) ; `gb/filter_<f>` = `<f>` ×8 de Super
    Mario Land, `480x432+176+592` (la pente de la pyramide, les palmiers et Mario).
- **`scenes`** : le vrai `Game`, en ×8 avec le filtre LCD, rendu par `Game.renderFrame`, la
  fonction du raccourci de capture. L'écran de reprise est pris avant la première image, avec
  la date du jour. L'écran titre est à l'image 200 pour Super Mario Land, et à l'image 900
  pour Link's Awakening DX, après Start à l'image 500. Le menu Pause est ouvert ensuite sur
  l'écran titre. Le harnais donne au menu un jeu récent : « Jeux recents... » est actif.
- **`drop`** : un `Game` sans ROM, en ×8. Il n'a pas de jeu, donc pas de filtre : il est
  dans la palette par défaut (DMG vert). Ses jeux récents sont, dans l'ordre, Link's
  Awakening DX (sélectionné), Super Mario Land, Tetris DX, Wario Land 3 et Donkey Kong
  Country. Leurs noms sont les titres lus dans l'en-tête des ROMs, comme gbe les affiche :
  ZELDA, SUPER MARIOLAND, TETRIS DX, WARIOLAND3, DK COUNTRY. Sur les cartouches Game Boy
  Color récentes, ce titre est limité à 11 caractères, sans le code fabricant qui le suit
  dans l'en-tête : Pokémon Pinball s'appelle « POKEMONPINB ». Pour changer la liste,
  préférer des jeux dont le titre reste lisible.

## Prérequis

- Les ROMs, absentes de git : dans `roms/`, `Super_Mario_Land_World_Rev1.gb`,
  `Legend_of_Zelda_The_Links_Awakening_DX.gbc`, `Legend_of_Zelda_The_Links_Awakening.gb`,
  `Tetris_World_Rev1.gb`, `Tetris_DX.zip`, `Wario_Land_3.zip` et `Donkey_Kong_Country.zip`.
  Si elles manquent, demander à l'utilisateur de les copier ; ne pas les chercher ailleurs.
- `go`, `cwebp` et `magick` (ImageMagick), plus Google Chrome pour l'aperçu du site.
- Une machine avec écran : le harnais ouvre des fenêtres Ebitengine quelques secondes.

## Démarche

### 1. Partir d'un `main` à jour

```sh
git checkout main && git pull --ff-only
git checkout -b feature/site-captures   # ou un nom plus précis
```

### 2. Prendre les captures

```sh
.claude/skills/site-captures/capture.sh <scratchpad>/captures [groupe...]
```

Sans groupe, le script les fait tous, en environ une minute. Avec des groupes (`hero`,
`palettes`, `colorized`, `filters`, `scenes`, `drop`), seulement ceux-là : par exemple
`scenes` après un changement du menu, `filters` après un changement d'un shader.

Écrire dans le scratchpad de la session, jamais dans le dépôt. Le script :

1. compile le harnais (`harness/capture.go` copié dans `internal/ui/zz_capture.go`,
   `harness/main.go` dans `zz_capture/main.go`), puis le retire du dépôt, même en cas
   d'erreur ;
2. prend les captures de chaque groupe demandé, selon les recettes ci-dessus. Le harnais a
   quatre modes :
   - par défaut, les trois scènes d'une ROM (`ui.Capture`) ;
   - `-drop`, l'écran de dépôt (`ui.CaptureDrop`) ;
   - `-shot`, une image d'une ROM par `ui.Screenshot`, avec `-frames`, `-press`,
     `-palette`, `-correct`, `-compat` et `-scale` ;
   - `-filters`, une image de 160×144 dessinée par chaque filtre à une taille donnée
     (`ui.CaptureFilters`) ;
3. convertit chaque PNG en WebP sans perte (`cwebp -lossless -z 9`) et produit
   `montage.png`, avec une ligne par groupe.

`SCALE`, `FILTER` et `UI_LANG` changent l'échelle, le filtre et la langue des groupes
`scenes` et `drop`, si l'utilisateur le demande ; par défaut ×8, `lcd` et `fr`. L'écran de
dépôt ignore le filtre. Les autres groupes ont des réglages fixes, dans `capture.sh`.

Le harnais ignore les manettes branchées sur la machine (`noPads`) : le menu s'affiche
toujours comme sans manette, sans « Start+Select: menu » dans son aide.

Si le harnais ne compile plus, c'est que l'API qu'il utilise a changé : `renderFrame`,
`menu.show`, `menu.showStart`, `started`, `stateTime`, `frame`, `Config.Recent`, `padReader`, la
construction de `Game` faite par `ui.Run`, `rom.Open`, `ui.Screenshot`, `ui.Palettes`,
`SetCompatPalette` ou `scaler.Pipeline`. Adapter `harness/`, et mettre le skill à jour
dans le même commit.

Après le script, `git status` ne doit montrer aucun `zz_capture` restant.

### 3. Vérifier les images

Comparer chaque nouvelle image à celle du site :

```sh
cd <out> && for f in gb/*.png gbc/*.png; do
  printf "%-28s %s\n" "${f%.png}" "$(magick compare -metric AE $f <repo>/screenshots/${f%.png}.webp null: 2>&1)"
done
```

Sans changement du code concerné, le résultat est 0 pour toutes les images, sauf :
- `save_state_resume`, dont la date change ;
- les captures qui dépendent de ce qui a changé : le menu, un shader, une palette…

Une différence inattendue signale un changement de l'émulation ou du rendu : la
comprendre avant d'installer.

Lire `montage.png` puis, au besoin, des détails recadrés en taille réelle :

```sh
magick <out>/gbc/menu.png -crop 640x400+320+380 +repage <out>/detail.png
```

Contrôler :
- les écrans titres sont affichés, et non l'intro ou un écran noir ;
- les palettes ont leurs couleurs, et chacune a son propre nom ;
- les détails des filtres montrent bien leur sujet : MARI, le bateau, la pente ;
- le menu est à jour (les entrées actuelles de `internal/ui/menu.go`) et son texte est
  net : en ×8, il est dessiné en ×3. « Jeux recents... » est actif, et non grisé ;
- la grille LCD est visible sur les scènes ;
- l'écran de reprise affiche « PARTIE EN COURS » et la date du jour ;
- l'écran de dépôt montre la cartouche, l'invitation « Deposez une ROM Game Boy ici » et
  les cinq jeux récents, ZELDA sélectionné, sans titre tronqué ni code fabricant.

Si une image a bougé, par exemple parce que l'émulation a changé, retrouver la bonne avec le
mode headless, puis ajuster l'image ou les appuis dans `capture.sh` :

```sh
go build -o <scratchpad>/gbe ./cmd/gbe
<scratchpad>/gbe -bios none -frames 900 -input "start:500-510" -screenshot <scratchpad>/t.png roms/Legend_of_Zelda_The_Links_Awakening_DX.gbc
```

### 4. Installer les images

Ne copier que les captures que l'utilisateur veut refaire, en général celles qui ont changé :

```sh
cp <out>/gb/drop_screen.webp screenshots/gb/drop_screen.webp   # par exemple
```

`save_state_resume` ne change que par sa date : ne la recopier que si autre chose a
changé.

Si l'échelle des scènes n'est plus ×8, mettre à jour dans `site/index.html.tmpl` :
- les attributs `width`/`height` des sept `<img>` concernées (160×échelle sur
  144×échelle) : les six de `#captures` et celle de `figure.drop-shot`, dans « Démarrage
  rapide » ;
- la phrase sous le titre des Captures, qui annonce l'échelle, la taille et le filtre.

De même si le filtre change. La figure de l'écran de dépôt a la largeur d'une colonne des
Captures (`.drop-shot` dans `site/style.css`), centrée : ne pas la changer sans en parler.
Si une recette change le sujet d'une image, mettre à jour son texte `alt` et sa légende.

### 5. Montrer l'aperçu du site

```sh
gh api -H "Accept: application/vnd.github.html+json" repos/ldechoux/gbe/releases > <scratchpad>/releases.json
go run ./tools/sitegen -releases <scratchpad>/releases.json -out <scratchpad>/www
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --headless=new --disable-gpu \
  --hide-scrollbars --force-device-scale-factor=1 --window-size=1280,16000 \
  --virtual-time-budget=5000 --screenshot=<scratchpad>/page.png "file://<scratchpad>/www/index.html"
```

Recadrer `page.png` autour des sections dont les images ont changé, pour les vérifier :
cadres, légendes sous les images, rien de décalé. Puis ouvrir
`<scratchpad>/www/index.html` dans le navigateur de l'utilisateur avec `open`.

### 6. Attendre la validation, puis faire la PR

Présenter à l'utilisateur ce qui a changé, image par image, avec le résultat de la
comparaison : scènes, réglages, tailles des fichiers, et modifications du template s'il y
en a. Ne commiter, pousser et ouvrir la PR qu'après son accord explicite.

- Le commit et la PR sont en anglais, comme le reste de l'historique. Le titre décrit le
  changement visible, par exemple « Retake the site captures with the LCD filter ».
- La PR liste les images refaites, les réglages, la méthode (harnais de ce skill, même rendu
  que gbe) et le plan de test (comparaison pixel à pixel, `go test ./tools/sitegen`, aperçu
  vérifié).
- Après le merge, le workflow GitHub Pages redéploie le site : `screenshots/**` et
  `site/**` le déclenchent.
