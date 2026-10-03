# gbe — émulateur Game Boy en Go pur

Émulateur Game Boy (DMG) et Game Boy Color (CGB) écrit en Go, sans cgo. Fenêtre, clavier et son reposent sur
[Ebitengine](https://ebitengine.org), qui passe par purego sur macOS et Windows.

Site du projet : https://ldechoux.github.io/gbe/

## Site du projet

Le site est généré par `tools/sitegen`, à partir de `site/`, des captures de `screenshots/` et des
releases GitHub. Le workflow [`pages.yml`](.github/workflows/pages.yml) le redéploie :

- après chaque release, une fois les binaires attachés ;
- quand une note de version est modifiée ou une release supprimée ;
- à chaque modification du site sur `main`.

Aperçu en local :

```sh
gh api -H "Accept: application/vnd.github.html+json" repos/ldechoux/gbe/releases > /tmp/releases.json
go run ./tools/sitegen -releases /tmp/releases.json -out /tmp/site && open /tmp/site/index.html
```

## Téléchargement

À chaque release GitHub publiée, le workflow [`release.yml`](.github/workflows/release.yml)
compile gbe et joint six archives à la release, sous la forme `gbe-<tag>-<os>-<arch>` :

| OS | Architectures |
|---|---|
| macOS | amd64, arm64 |
| Linux | amd64, arm64 |
| Windows | amd64, arm64 |

Remarques par plateforme :

- **macOS** : les binaires ne sont pas signés. Au premier lancement, faire clic droit >
  Ouvrir, ou lancer `xattr -d com.apple.quarantine gbe`.
- **Linux** : il faut `libX11`, `libGL` et `libasound`, présents sur tout bureau standard.

`gbe -version` affiche la version.

## Lancer

Avec Go 1.27 ou plus récent, gbe s'installe directement (sans cgo) :

```sh
go install github.com/ldechoux/gbe/cmd/gbe@latest
gbe mon-jeu.gb
```

Le binaire est installé dans `$(go env GOPATH)/bin`.

Depuis les sources :

```sh
CGO_ENABLED=0 go build -o bin/gbe ./cmd/gbe
./bin/gbe roms/Super_Mario_Land_World_Rev1.gb
```

Les dossiers `bios/` (boot ROM) et `roms/` ne sont pas versionnés : les fichiers qu'ils
contiennent sont sous copyright. Il faut y placer ses propres copies.

Une ROM compressée se lance directement (`./bin/gbe roms/Tetris_DX.zip`) : la première
ROM `.gb` ou `.gbc` de l'archive est décompressée en mémoire, et les sauvegardes
(`Tetris_DX.sav`, `Tetris_DX.state`) sont enregistrées à côté de l'archive, qui n'est
jamais modifiée.

Options utiles :

| Option | Rôle |
|---|---|
| `-bios chemin` | Boot ROM à exécuter (défaut `bios/gb_bios.bin`, ou `bios/gbc_bios.bin` en mode Game Boy Color ; ignorée si absente ; `none` pour démarrer directement le jeu) |
| `-model auto\|gb\|gbc` | Matériel émulé. `auto` (défaut) choisit celui pour lequel le jeu a été fait, d'après l'octet 0x0143 de l'en-tête : la Game Boy Color pour les jeux qui la gèrent, la Game Boy d'origine pour les autres, sauf si la colorisation est activée dans le menu. `gb` force la Game Boy d'origine (palettes monochromes, ou un jeu compatible avec les deux), `gbc` la Game Boy Color. `dmg` et `cgb`, les noms du matériel chez Nintendo, sont acceptés aussi |
| `-scale N` | Taille de la fenêtre (1 à 8) |
| `-screenshot-dir chemin` | Dossier des captures d'écran (défaut `~/Pictures/gbe`) |
| `-config chemin` | Fichier de config (défaut `~/Library/Application Support/gbe/config.json` sur macOS) |

## Commandes

| Touche | Action |
|---|---|
| Flèches | Croix directionnelle |
| X / Z (X / W en AZERTY) | A / B |
| Entrée / Maj droite | Start / Select |
| Tab (maintenu) | Avance rapide (x4 par défaut, réglable de x2 à x8 dans le menu) |
| Retour arrière (maintenu) | Rembobinage : le jeu revient en arrière, jusqu'à 10 s |
| Échap | Menu (pause) |
| P | Palette suivante (sur une DMG, ou palette GBC d'un jeu DMG colorisé), ou correction des couleurs dans un jeu Game Boy Color (si P n'est pas assigné à un bouton) |
| Cmd+F2 (Ctrl+F2 hors macOS) | Capture d'écran PNG |
| F11 | Plein écran |

Le menu permet de changer la palette (10 palettes monochromes, aperçu en direct ; dans un
jeu Game Boy Color, cette entrée active ou non la correction des couleurs, qui imite l'écran
d'origine, plus pâle, et le menu s'affiche toujours en noir et blanc ; pour un jeu DMG
colorisé, voir ci-dessous), d'activer ou non la colorisation des jeux DMG, de
redéfinir chaque touche (Entrée sur un bouton puis appuyer sur la nouvelle touche ; en cas
de conflit, les deux touches sont échangées), de régler le volume, l'échelle, la vitesse
de l'avance rapide et la langue, de réinitialiser la console ou de quitter.

Pendant l'avance rapide, le son est accéléré lui aussi. Le rembobinage recule deux fois plus
vite que le jeu n'avance, sans son, et s'arrête sur la plus ancienne image gardée (10 s de jeu,
ce qui occupe environ 30 Mo de mémoire). Un indicateur `>> x4` ou `<<` s'affiche en haut à
droite. Les touches de ces deux actions se redéfinissent dans la page Contrôles, comme celles
des boutons (lignes « Avance » et « Arriere »).

Le raccourci de capture se change dans la même page Contrôles : sélectionner « Capture »,
puis appuyer sur la combinaison voulue (modificateurs compris). Les captures sont
enregistrées à la taille de la fenêtre (échelle choisie), avec la palette courante, sous la
forme `<titre>-AAAAMMJJ-HHMMSS.png`. Chaque réglage est enregistré immédiatement.

Les touches sont enregistrées par position physique : un mapping reste valable si l'on change de
disposition de clavier. Le menu affiche leur nom selon la disposition active (AZERTY, QWERTZ…).

## Jeux DMG en couleurs

Comme une vraie Game Boy Color, gbe peut coloriser les jeux Game Boy. C'est une option :
par défaut, ils tournent sur la Game Boy d'origine. Pour l'activer, passer l'entrée
« Coloriser » du menu à « oui » (elle apparaît pour les jeux Game Boy lancés sans `-model`)
puis « Réinitialiser », ou lancer le jeu avec `-model gbc` :

- **Palette automatique** : les jeux Nintendo reçoivent la palette que la boot ROM couleur
  prévoit pour leur titre (Tetris en jaune et rouge, Link's Awakening en rose…), les autres
  une palette par défaut.
- **12 palettes au choix** : celles que la console offrait en maintenant une direction, seule
  ou avec A ou B, pendant le logo. L'entrée Palette du menu et la touche P passent de « Auto »
  à « Droite », « Gauche+A »… avec aperçu en direct ; le choix est retenu pour chaque jeu.
- **Boot ROM** : avec `bios/gbc_bios.bin`, l'animation de démarrage et les combinaisons au
  logo fonctionnent comme sur la console. Sans elle, gbe reprend les tables de la boot ROM
  (vérifiées contre l'original par les tests).
- **Fidélité** : la console tourne dans le mode de compatibilité de la Game Boy Color
  (registre KEY0) ; la correction des couleurs s'applique comme pour les jeux GBC.
- **Changer de réglage** : passer à une autre console demande de redémarrer le jeu. Tant que
  le réglage et le mode en cours diffèrent, le bas du menu indique « Reinitialiser pour
  appliquer » ; « Réinitialiser » redémarre alors dans le mode choisi (la partie en cours est
  perdue, pas le `.sav`).

## Langues

L'interface est disponible en anglais (par défaut) et en français. La langue se choisit dans
le menu (« Language » / « Langue ») et est enregistrée dans la config (clé `language`).

Pour ajouter une langue, copier `internal/i18n/locales/en.json` en `<code>.json` (par exemple
`es.json`), renseigner `name` (nom de la langue dans cette langue) et traduire les messages,
puis recompiler : la langue apparaît automatiquement dans le menu. Les traductions doivent
rester en ASCII (la police du menu n'a pas d'accents) ; `go test ./internal/i18n` vérifie
qu'aucun message ne manque.

## Manettes

Les manettes reconnues par la base SDL intégrée à Ebitengine (Xbox, PlayStation, Switch Pro et
la plupart des manettes USB/Bluetooth) fonctionnent dès qu'elles sont branchées. Par défaut :

| Manette | Game Boy |
|---|---|
| Croix ou stick gauche | Croix directionnelle |
| Bouton de droite / du bas (B / A sur Xbox, Rond / Croix sur PlayStation) | A / B |
| Start / Select (Menu / Vue, Options / Share, + / −) | Start / Select |
| Gâchette haute droite / gauche (RB / LB, R1 / L1, R / L), maintenue | Avance rapide / rembobinage |
| Start + Select ensemble | Menu (croix pour naviguer, A pour valider, B pour revenir) |

Les boutons et les deux actions se redéfinissent dans l'onglet **Manette** de la page Contrôles (←/→ pour changer
d'onglet). Il est grisé tant qu'aucune manette n'est connectée. Les libellés affichés suivent la
famille de la manette détectée : Xbox, PlayStation ou Nintendo.

## Save states

- **À la fermeture** : quand on quitte (menu « Quitter » ou fermeture de la fenêtre), l'état
  complet de la console est enregistré dans `<rom>.state`, à côté de la ROM.
- **Au lancement suivant** : l'émulateur propose de reprendre la partie ou de recommencer
  depuis le début.
- **Pendant le jeu** : le menu permet aussi de sauvegarder ou de recharger l'état à tout
  moment.
- **Sécurité** : un état fait avec une autre ROM (ou une autre version), ou dans l'autre mode
  (DMG / Game Boy Color), est refusé. Les états des versions précédentes restent lisibles.
- **Changement de mode** : avec `-model auto`, une partie sauvegardée dans l'autre mode (par
  exemple une partie DMG faite avant d'activer la colorisation) se reprend dans son mode ;
  l'écran de reprise le signale sous la date, et « Recommencer » relance la console dans le
  mode choisi. Le `.sav` vaut pour les deux modes.

## Compatibilité

- Cartouches : ROM seule, MBC1, MBC2, MBC3 (avec RTC), MBC5.
- Les cartouches à pile sont sauvegardées dans `<rom>.sav`, à la fermeture et toutes les 5 s.
  L'horloge MBC3 est enregistrée au format BGB/VBA-M.
- Game Boy Color : RAM et VRAM en banques, palettes couleur, attributs des tiles, priorités
  CGB, DMA VRAM (général et HBlank), double vitesse, boot ROM CGB, et mode de compatibilité
  qui colorise les jeux DMG.
- Tests réussis : blargg `cpu_instrs`, `instr_timing`, `mem_timing` (en DMG et en CGB),
  `halt_bug`, `dmg-acid2` et `cgb-acid2` au pixel près, et les 23 tests son de blargg
  (`dmg_sound`, `cgb_sound`).

## Tests

```sh
mkdir -p testroms && cd testroms
for f in cpu_instrs/cpu_instrs.gb instr_timing/instr_timing.gb mem_timing/mem_timing.gb; do
  curl -sfLO "https://github.com/retrio/gb-test-roms/raw/master/$f"
done
curl -sfLO https://github.com/mattcurrie/dmg-acid2/releases/download/v1.0/dmg-acid2.gb
curl -sfL -o dmg-acid2-ref.png https://raw.githubusercontent.com/mattcurrie/dmg-acid2/master/img/reference-dmg.png
curl -sfLO https://github.com/mattcurrie/cgb-acid2/releases/download/v1.1/cgb-acid2.gbc
curl -sfL -o cgb-acid2-ref.png https://raw.githubusercontent.com/mattcurrie/cgb-acid2/master/img/reference.png
for set in dmg_sound cgb_sound; do
  mkdir -p $set
  for n in 01-registers "02-len ctr" 03-trigger 04-sweep "05-sweep details" "06-overflow on trigger" \
           "07-len sweep period sync" "08-len ctr during power" "09-wave read while on" \
           "10-wave trigger while on" "11-regs after power" "12-wave write while on"; do
    curl -sfL -o "$set/$n.gb" "https://github.com/retrio/gb-test-roms/raw/master/$set/rom_singles/${n// /%20}.gb" || true
  done
done
cd .. && go test ./...
```

Sans ces ROMs, les tests qui en dépendent sont ignorés.

La ROM de benchmark ([`benchrom/`](benchrom/README.md)), écrite pour gbe, est dans le dépôt :
ses tests tournent partout, la CI comprise. Ses scènes reproduisent la charge de types de
jeux (jeu classique, calcul, jeu presque toujours en pause, Game Boy Color, son). Pour chacune,
en DMG et en CGB, l'image finale doit correspondre à une image de référence. En cas d'écart,
`GBE_GOLDEN_OUT=<dossier>` récupère l'image et le son obtenus.

Les tests « golden » (`internal/gb/golden_test.go`) hashent le son, chaque image et l'état
final de la machine sur la ROM de benchmark, un programme synthétique, les ROMs de test et
quelques jeux gardés dans `roms/` : une optimisation ne doit pas changer un seul bit.

Un mode sans fenêtre sert au débogage :
`./bin/gbe -frames 600 -input "start:400-410" -screenshot out.png -wav out.wav jeu.gb`.

## Performances

Sur chaque pull request, la CI lance les benchmarks de la branche et ceux de sa branche de
base, en alternance sur la même machine. Le rapport des écarts est publié en commentaire de la
PR, mis à jour à chaque push : médianes, test statistique, 🟢 au-delà de 5 % plus rapide, ⚠️
au-delà de 10 % plus lent. Il est informatif : les machines de la CI sont partagées, donc
bruitées, et il ne fait jamais échouer la PR.

La même comparaison en local, par exemple avec `main` :

```sh
git worktree add /tmp/gbe-main main
tools/benchcompare.sh /tmp/gbe-main . 10 /tmp/bench
go run ./tools/benchreport -base main -head branche /tmp/bench/base.txt /tmp/bench/head.txt
```

Le binaire est compilé avec l'optimisation guidée par profil (PGO) : `go build` utilise
automatiquement `cmd/gbe/default.pgo`. Le régénérer après un changement important du cœur :

```sh
go test ./internal/gb -run '^$' -bench Frame -benchtime 3s -cpuprofile cmd/gbe/default.pgo
```

## Architecture

- `internal/gb` : le cœur, sans dépendance graphique.
  - CPU SM83 : chaque accès mémoire fait avancer le reste de la machine d'un M-cycle.
  - PPU : rendu ligne par ligne, en teintes (DMG) ou en couleurs RGB555 (CGB).
  - APU : 4 canaux, filtre passe-haut.
  - Timer, joypad, port série, MBC.
- `internal/ui` : le frontend Ebitengine.
  - Rendu avec la palette choisie, ou en couleurs (corrigées ou non) en mode Game Boy Color.
  - Audio : tampon dont le taux d'échantillonnage s'ajuste légèrement pour compenser l'écart
    entre 60 Hz et 59,73 Hz.
  - Menu, configuration JSON.
- `internal/i18n` : les traductions de l'interface (fichiers JSON embarqués).
- `cmd/gbe` : le point d'entrée.
