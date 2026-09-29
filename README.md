# gbe — émulateur Game Boy en Go pur

Émulateur Game Boy (DMG) écrit en Go, sans cgo. Fenêtre, clavier et son reposent sur
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

Options utiles :

| Option | Rôle |
|---|---|
| `-bios chemin` | Boot ROM à exécuter (défaut `bios/gb_bios.bin`, ignorée si absente ; `none` pour démarrer directement le jeu) |
| `-scale N` | Taille de la fenêtre (1 à 8) |
| `-screenshot-dir chemin` | Dossier des captures d'écran (défaut `~/Pictures/gbe`) |
| `-config chemin` | Fichier de config (défaut `~/Library/Application Support/gbe/config.json` sur macOS) |

## Commandes

| Touche | Action |
|---|---|
| Flèches | Croix directionnelle |
| X / Z (X / W en AZERTY) | A / B |
| Entrée / Maj droite | Start / Select |
| Échap | Menu (pause) |
| P | Palette suivante (si P n'est pas assigné à un bouton) |
| Cmd+F2 (Ctrl+F2 hors macOS) | Capture d'écran PNG |
| F11 | Plein écran |

Le menu permet de changer la palette (10 palettes monochromes, aperçu en direct), de
redéfinir chaque touche (Entrée sur un bouton puis appuyer sur la nouvelle touche ; en cas
de conflit, les deux touches sont échangées), de régler le volume et l'échelle, de
réinitialiser la console ou de quitter.

Le raccourci de capture se change dans la même page Contrôles : sélectionner « Capture »,
puis appuyer sur la combinaison voulue (modificateurs compris). Les captures sont
enregistrées à la taille de la fenêtre (échelle choisie), avec la palette courante, sous la
forme `<titre>-AAAAMMJJ-HHMMSS.png`. Chaque réglage est enregistré immédiatement.

Les touches sont enregistrées par position physique : un mapping reste valable si l'on change de
disposition de clavier. Le menu affiche leur nom selon la disposition active (AZERTY, QWERTZ…).

## Save states

- **À la fermeture** : quand on quitte (menu « Quitter » ou fermeture de la fenêtre), l'état
  complet de la console est enregistré dans `<rom>.state`, à côté de la ROM.
- **Au lancement suivant** : l'émulateur propose de reprendre la partie ou de recommencer
  depuis le début.
- **Pendant le jeu** : le menu permet aussi de sauvegarder ou de recharger l'état à tout
  moment.
- **Sécurité** : un état fait avec une autre ROM (ou une autre version) est refusé.

## Compatibilité

- Cartouches : ROM seule, MBC1, MBC2, MBC3 (avec RTC), MBC5.
- Les cartouches à pile sont sauvegardées dans `<rom>.sav`, à la fermeture et toutes les 5 s.
  L'horloge MBC3 est enregistrée au format BGB/VBA-M.
- Tests réussis : blargg `cpu_instrs`, `instr_timing`, `mem_timing`, `halt_bug`, et
  `dmg-acid2` au pixel près.

## Tests

```sh
mkdir -p testroms && cd testroms
for f in cpu_instrs/cpu_instrs.gb instr_timing/instr_timing.gb mem_timing/mem_timing.gb; do
  curl -sfLO "https://github.com/retrio/gb-test-roms/raw/master/$f"
done
curl -sfLO https://github.com/mattcurrie/dmg-acid2/releases/download/v1.0/dmg-acid2.gb
curl -sfL -o dmg-acid2-ref.png https://raw.githubusercontent.com/mattcurrie/dmg-acid2/master/img/reference-dmg.png
cd .. && go test ./...
```

Sans ces ROMs, les tests qui en dépendent sont ignorés.

Un mode sans fenêtre sert au débogage :
`./bin/gbe -frames 600 -input "start:400-410" -screenshot out.png -wav out.wav jeu.gb`.

## Architecture

- `internal/gb` : le cœur, sans dépendance graphique.
  - CPU SM83 : chaque accès mémoire fait avancer le reste de la machine d'un M-cycle.
  - PPU : rendu ligne par ligne.
  - APU : 4 canaux, filtre passe-haut.
  - Timer, joypad, port série, MBC.
- `internal/ui` : le frontend Ebitengine.
  - Rendu avec la palette choisie.
  - Audio : tampon dont le taux d'échantillonnage s'ajuste légèrement pour compenser l'écart
    entre 60 Hz et 59,73 Hz.
  - Menu, configuration JSON.
- `cmd/gbe` : le point d'entrée.
