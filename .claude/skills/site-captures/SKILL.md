---
name: site-captures
description: Refait les captures du site du projet avec le rendu de capture de gbe, à l'échelle ×8 — les six de la section « Captures » (écran titre, menu Pause et écran de reprise, en Game Boy et en Game Boy Color, filtre LCD) et l'écran de dépôt des ROMs du « Démarrage rapide » — puis prépare l'aperçu du site. À utiliser quand l'utilisateur demande de refaire, mettre à jour ou régénérer les captures du site (screenshots/gb, screenshots/gbc), par exemple après un changement du menu, d'un filtre ou d'une palette.
---

# Refaire les captures du site

Le site (`site/index.html.tmpl`) montre sept captures de `screenshots/` :

| Fichier | Scène | Section du site |
|---|---|---|
| `screenshots/{gb,gbc}/game_title.webp` | L'écran titre du jeu | Captures (`#captures`) |
| `screenshots/{gb,gbc}/menu.webp` | Le menu Pause (en français, curseur sur Reprendre) par-dessus l'écran titre | Captures |
| `screenshots/{gb,gbc}/save_state_resume.webp` | L'écran de reprise de partie, au lancement, sur l'écran encore vide | Captures |
| `screenshots/gb/drop_screen.webp` | L'écran de dépôt des ROMs, affiché quand gbe est lancé sans ROM, avec 5 jeux récents | Télécharger, « Démarrage rapide » (`figure.drop-shot`) |

Les jeux sont Super Mario Land (Game Boy, palette DMG vert) et Link's Awakening DX (Game Boy
Color, couleurs corrigées). Les réglages : échelle ×8 (1280×1152), filtre LCD, interface en
français. Les images sont produites par `Game.renderFrame`, la fonction du raccourci de
capture : image filtrée, et menu par-dessus quand il est ouvert.

L'écran de dépôt n'a pas de jeu, donc pas de filtre : il est dans la palette par défaut
(DMG vert). Ses jeux récents sont, dans l'ordre, Link's Awakening DX (sélectionné), Super
Mario Land, Tetris DX, Wario Land 3 et Donkey Kong Country. Leurs noms sont les titres lus
dans l'en-tête des ROMs, comme gbe les affiche : ZELDA, SUPER MARIOLAND, TETRIS DX,
WARIOLAND3, DK COUNTRY. Éviter Pokémon Pinball, dont l'en-tête donne « POKEMONPINBVPHP »
(titre suivi du code fabricant), tant que la lecture du titre ne le corrige pas.

## Prérequis

- Les ROMs, absentes de git : `roms/Super_Mario_Land_World_Rev1.gb`,
  `roms/Legend_of_Zelda_The_Links_Awakening_DX.gbc`, et pour les jeux récents de l'écran
  de dépôt `roms/Tetris_DX.zip`, `roms/Wario_Land_3.zip` et `roms/Donkey_Kong_Country.zip`.
  Si elles manquent, demander à l'utilisateur de les copier ; ne pas les chercher ailleurs.
- `go`, `cwebp` et `magick` (ImageMagick), plus Google Chrome pour l'aperçu du site.
- Une machine avec écran : le harnais ouvre une fenêtre Ebitengine quelques secondes.

## Démarche

### 1. Partir d'un `main` à jour

```sh
git checkout main && git pull --ff-only
git checkout -b feature/site-captures   # ou un nom plus précis
```

### 2. Prendre les captures

```sh
.claude/skills/site-captures/capture.sh <scratchpad>/captures
```

Écrire dans le scratchpad de la session, jamais dans le dépôt. Le script :

1. copie le harnais (`harness/capture.go` dans `internal/ui/zz_capture.go`,
   `harness/main.go` dans `zz_capture/main.go`) ;
2. lance le vrai `Game` pour chaque console et enregistre les trois scènes :
   - l'écran de reprise avant la première image, avec la date du jour ;
   - l'écran titre : image 200 pour Super Mario Land ; image 900 pour Link's Awakening
     DX, après un appui sur Start à l'image 500 pour passer l'intro ;
   - le menu Pause, ouvert ensuite sur l'écran titre ;
3. lance un `Game` sans ROM (`-drop`, `ui.CaptureDrop`) et enregistre
   `gb/drop_screen.png`. Les cinq ROMs de la liste `recent` du script sont ouvertes avec
   `rom.Open` pour lire leur titre, et deviennent les jeux récents, le premier sélectionné ;
4. retire le harnais, même en cas d'erreur, puis convertit les PNG en WebP sans perte
   (`cwebp -lossless -z 9`) et produit `montage.png`.

`SCALE`, `FILTER` et `UI_LANG` changent l'échelle, le filtre et la langue si l'utilisateur
le demande ; par défaut ×8, `lcd` et `fr`. L'écran de dépôt suit l'échelle et la langue ;
il ignore le filtre.

Si le harnais ne compile plus, c'est que les champs de `Game` ou de `menu` ont changé :
adapter `harness/capture.go`, qui utilise `renderFrame`, `menu.show`, `menu.showStart`,
`started`, `stateTime`, `frame`, `Config.Recent`, `rom.Open` et la construction de `Game`
faite par `ui.Run`. Mettre
le skill à jour dans le même commit.

Après le script, `git status` ne doit montrer aucun `zz_capture` restant.

### 3. Vérifier les images

Lire `montage.png` puis, au besoin, des détails recadrés en taille réelle :

```sh
magick <out>/gbc/menu.png -crop 640x400+320+380 +repage <out>/detail.png
```

Contrôler :
- l'écran titre est affiché, et non l'intro ou un écran noir ;
- le menu est à jour (les entrées actuelles de `internal/ui/menu.go`) et son texte est
  net : en ×8, il est dessiné en ×3. « Jeux recents... » est actif, et non grisé : le
  harnais donne au menu un jeu récent, comme pour tout joueur qui a déjà joué ;
- la grille LCD est visible ;
- l'écran de reprise affiche « PARTIE EN COURS » et la date du jour ;
- l'écran de dépôt montre la cartouche, l'invitation « Deposez une ROM Game Boy ici » et
  les cinq jeux récents, ZELDA sélectionné, sans titre tronqué ni code fabricant.

Si l'écran titre de Link's Awakening DX a bougé, par exemple parce que l'émulation a
changé, trouver la bonne image avec le mode headless :

```sh
go build -o <scratchpad>/gbe ./cmd/gbe
<scratchpad>/gbe -bios none -frames 900 -input "start:500-510" -screenshot <scratchpad>/t.png roms/Legend_of_Zelda_The_Links_Awakening_DX.gbc
```

Puis ajuster `-title-at` ou `-start-at` dans `capture.sh`.

### 4. Installer les images

```sh
for m in gb gbc; do for f in game_title menu save_state_resume; do
  cp <out>/$m/$f.webp screenshots/$m/$f.webp
done; done
cp <out>/gb/drop_screen.webp screenshots/gb/drop_screen.webp
```

Ne copier que les captures que l'utilisateur veut refaire : par exemple seulement
`drop_screen.webp` après un changement de l'écran de dépôt.

Si l'échelle n'est plus ×8, mettre à jour dans `site/index.html.tmpl` les attributs
`width`/`height` des sept `<img>` (160×échelle sur 144×échelle) : les six de `#captures`
et celle de `figure.drop-shot`, dans « Démarrage rapide ». Mettre aussi à jour la phrase
sous le titre des Captures, qui annonce l'échelle, la taille et le filtre. De même si le
filtre change. La figure de l'écran de dépôt a la largeur d'une colonne des Captures
(`.drop-shot` dans `site/style.css`), centrée : ne pas la changer sans en parler.

### 5. Montrer l'aperçu du site

```sh
gh api -H "Accept: application/vnd.github.html+json" repos/ldechoux/gbe/releases > <scratchpad>/releases.json
go run ./tools/sitegen -releases <scratchpad>/releases.json -out <scratchpad>/www
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --headless=new --disable-gpu \
  --hide-scrollbars --force-device-scale-factor=1 --window-size=1280,16000 \
  --virtual-time-budget=5000 --screenshot=<scratchpad>/page.png "file://<scratchpad>/www/index.html"
```

Recadrer `page.png` autour de la section Captures, et du « Démarrage rapide » pour
l'écran de dépôt, pour les vérifier : cadres, légendes sous les images, rien de décalé. Puis ouvrir `<scratchpad>/www/index.html` dans le
navigateur de l'utilisateur avec `open`.

### 6. Attendre la validation, puis faire la PR

Présenter à l'utilisateur ce qui a changé : scènes, réglages, tailles des fichiers, et
modifications du template s'il y en a. Ne commiter, pousser et ouvrir la PR qu'après son
accord explicite.

- Le commit et la PR sont en anglais, comme le reste de l'historique. Le titre décrit le
  changement visible, par exemple « Retake the site captures with the LCD filter ».
- La PR liste les scènes refaites, les réglages, la méthode (harnais de ce skill, même rendu
  que le raccourci de capture) et le plan de test (`go test ./tools/sitegen`, aperçu
  vérifié).
- Après le merge, le workflow GitHub Pages redéploie le site : `screenshots/**` et
  `site/**` le déclenchent.
