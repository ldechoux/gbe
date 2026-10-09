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
compile gbe et joint à la release :
- **pour macOS, l'application** `gbe.app`, pour les Mac Intel et Apple Silicon, dans une
  image disque `gbe-<tag>-macos-universal.dmg` ;
- **pour Linux, une AppImage** par architecture, `gbe-<tag>-linux-<arch>.AppImage` ;
- **six archives du binaire seul**, à lancer depuis un terminal, sous la forme
  `gbe-<tag>-<os>-<arch>` :

| OS | Architectures |
|---|---|
| macOS | amd64, arm64 |
| Linux | amd64, arm64 |
| Windows | amd64, arm64 |

Remarques par plateforme :

- **macOS** : ouvrir l'image disque et glisser gbe dans Applications. L'application n'est
  pas signée par Apple, faute de compte Apple Developer. Au premier lancement, macOS
  refuse de l'ouvrir :
  1. fermer le message ;
  2. ouvrir Réglages Système > Confidentialité et sécurité, puis cliquer sur « Ouvrir quand
     même » en face de gbe, et confirmer.

  Les lancements suivants se font normalement. En variante, dans un terminal :
  `xattr -dr com.apple.quarantine /Applications/gbe.app`. Pour le binaire seul :
  `xattr -d com.apple.quarantine gbe`.
- **Windows** : décompresser l'archive et double-cliquer sur `gbe.exe`, qui a l'icône de
  gbe. L'exécutable n'est pas signé : au premier lancement, Windows SmartScreen peut
  afficher « Windows a protégé votre ordinateur ». Cliquer sur « Informations
  complémentaires », puis sur « Exécuter quand même ».
- **Linux** : l'AppImage `gbe-<tag>-linux-<arch>.AppImage` est un seul fichier, avec
  l'icône et l'entrée de menu de gbe. La rendre exécutable (Propriétés > Permissions, ou
  `chmod +x`), puis la lancer d'un double-clic. Les archives contiennent aussi `gbe.desktop`
  et `gbe.png`, pour installer l'entrée de menu à la main. Il faut `libX11`, `libGL` et
  `libasound`, présents sur tout bureau standard.

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

Le plus simple est de choisir le dossier des boot ROM `gb_bios.bin` et `gbc_bios.bin` dans
le menu : la page « Boot ROM... » du menu Pause ouvre le dialogue « Choisir un dossier » du
système et indique si chaque boot ROM est trouvée. On peut aussi déposer le dossier sur la
fenêtre pendant que la page est ouverte, par exemple sous Linux sans `zenity`, `qarma` ni
`matedialog`, que le dialogue utilise. « Recherche auto » oublie le dossier choisi. Pendant
une partie, le nouveau dossier s'applique à la prochaine réinitialisation.

gbe cherche les boot ROM dans cet ordre :
- le dossier choisi dans le menu ;
- le dossier `bios` du dossier courant ;
- le dossier `bios` du dossier de configuration (`~/Library/Application Support/gbe/bios`
  sur macOS, `%AppData%\gbe\bios` sous Windows, `~/.config/gbe/bios` sous Linux) : il est
  trouvé quel que soit l'endroit d'où gbe est lancé, y compris depuis le Finder ou
  l'Explorateur ;
- le dossier `bios` à côté de l'exécutable.

gbe écrit aussi ses messages dans `gbe.log`, dans le dossier de configuration, pour garder
une trace quand il est lancé sans terminal. Le fichier repart de zéro à chaque lancement. Une ROM passée en argument qui
ne s'ouvre pas est expliquée sur l'écran d'accueil.

Une ROM compressée se lance directement (`./bin/gbe roms/Tetris_DX.zip`) : la première
ROM `.gb` ou `.gbc` de l'archive est décompressée en mémoire, et les sauvegardes
(`Tetris_DX.sav`, `Tetris_DX.state`) sont enregistrées à côté de l'archive, qui n'est
jamais modifiée.

### Glisser-déposer une ROM

On peut aussi déposer une ROM (`.gb`, `.gbc` ou `.zip`) sur la fenêtre de gbe :

- **Lancé sans ROM** (`gbe`, ou un double-clic sur le binaire), gbe ouvre un écran qui
  invite à déposer une ROM. Le jeu démarre dès qu'on la dépose. Le menu (Échap, ou
  Start+Select à la manette) reste disponible pour régler l'affichage, les contrôles ou la
  langue, et P change la palette.
- **Les 5 derniers jeux lancés** sont proposés sur ce même écran : on en choisit un avec
  ↑/↓ (ou la croix) et on le lance avec Entrée (ou A). En cours de partie, la même liste est
  dans la page « Jeux récents » du menu. C'est utile à la manette, sur une TV, où l'on ne
  peut pas déposer de fichier. Un jeu qui n'existe plus est retiré de la liste.
- **Pendant une partie**, la page « Nouveau jeu » demande s'il faut lancer le jeu déposé ou
  continuer à jouer. Avant de changer de jeu, la partie en cours est sauvegardée comme en
  quittant (save state et sauvegarde de la cartouche) : on la retrouvera en rouvrant ce jeu.
- **Les sauvegardes** d'un jeu déposé sont enregistrées à côté de la ROM, comme en ligne de
  commande. Si ce jeu a un save state, gbe propose de reprendre la partie.
- **Les options** `-model` et `-bios` s'appliquent aussi aux jeux déposés.

Si le fichier ne peut pas être lancé, un message l'explique et la partie en cours continue :
fichier qui n'est pas une ROM, dossier, archive sans ROM, cartouche non supportée (avec son
type, par exemple `0x22`), fichier illisible. Si plusieurs fichiers sont déposés, gbe lance
la première ROM.

Options utiles :

| Option | Rôle |
|---|---|
| `-bios chemin` | Boot ROM à exécuter (défaut `gb_bios.bin`, ou `gbc_bios.bin` en mode Game Boy Color, cherchée dans le dossier choisi dans le menu puis dans les dossiers `bios` décrits plus haut ; ignorée si absente ; `none` pour démarrer directement le jeu) |
| `-model auto\|gb\|gbc` | Matériel émulé. `auto` (défaut) choisit celui pour lequel le jeu a été fait, d'après l'octet 0x0143 de l'en-tête : la Game Boy Color pour les jeux qui la gèrent, la Game Boy d'origine pour les autres, sauf si la colorisation est activée dans le menu. `gb` force la Game Boy d'origine (palettes monochromes, ou un jeu compatible avec les deux), `gbc` la Game Boy Color. `dmg` et `cgb`, les noms du matériel chez Nintendo, sont acceptés aussi |
| `-scale N` | Taille de la fenêtre (1 à 8) |
| `-filter nom` | Filtre d'affichage : `nearest`, `sharp`, `lcd`, `scale2x`, `scale3x` ou `mmpx` (voir [Affichage](#affichage-grands-écrans)) |
| `-stereo nom` | Sortie du son : `stereo`, `headphones` (casque) ou `mono` (voir [Son](#son)) |
| `-audio-filter nom` | Filtre audio : `off`, `soft`, `warm` ou `speaker` (voir [Son](#son)) |
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
| F11 | Plein écran (retenu au prochain lancement ; aussi dans la page Affichage, entrée « Mode ») |

Le menu permet de régler la vitesse de l'avance rapide et la langue, de sauvegarder ou
recharger l'état, de réinitialiser la console ou de quitter. Sa page **Jeux récents** relance
l'un des 5 derniers jeux (la partie en cours est sauvegardée d'abord). Trois pages regroupent
les autres réglages :

- **Affichage** : la palette (10 palettes monochromes, aperçu en direct ; dans un jeu Game
  Boy Color, cette entrée active ou non la correction des couleurs, qui imite l'écran
  d'origine, plus pâle, et le menu s'affiche toujours en noir et blanc ; pour un jeu DMG
  colorisé, voir ci-dessous), la colorisation des jeux DMG, l'échelle de la fenêtre, l'écran
  où elle s'ouvre (voir [Jouer sur la TV](#jouer-sur-la-tv)), le plein écran ou la fenêtre,
  le filtre d'affichage et la rémanence.
- **Son** : le volume, la sortie (stéréo, casque ou mono), le filtre audio et les bruitages
  des menus (voir [Son](#son)).
- **Contrôles** : chaque touche se redéfinit (Entrée sur un bouton puis appuyer sur la
  nouvelle touche ; en cas de conflit, les deux touches sont échangées).

Pendant l'avance rapide, le son est accéléré lui aussi. Le rembobinage recule deux fois plus
vite que le jeu n'avance, sans son, et s'arrête sur la plus ancienne image gardée (10 s de jeu,
ce qui occupe environ 30 Mo de mémoire). Un indicateur `>> x4` ou `<<` s'affiche en haut à
droite. Les touches de ces deux actions se redéfinissent dans la page Contrôles, comme celles
des boutons (lignes « Avance » et « Arriere »).

Le raccourci de capture se change dans la même page Contrôles : sélectionner « Capture »,
puis appuyer sur la combinaison voulue (modificateurs compris). Les captures sont
enregistrées à la taille de la fenêtre (échelle choisie), avec la palette, le filtre
d'affichage et la rémanence courants, et le menu s'il est ouvert (mais sans les
notifications), sous la forme `<titre>-AAAAMMJJ-HHMMSS.png`. Chaque réglage est enregistré
immédiatement.

Les touches sont enregistrées par position physique : un mapping reste valable si l'on change de
disposition de clavier. Le menu affiche leur nom selon la disposition active (AZERTY, QWERTZ…).

## Affichage (grands écrans)

L'entrée « Filtre » de la page Affichage du menu choisit comment l'image de 160×144 pixels
est agrandie. Tous les filtres tournent sur la carte graphique (shaders Kage), et l'image
reste centrée avec ses proportions :

| Filtre | Rendu |
|---|---|
| Pixels entiers (`nearest`, défaut) | Pixels carrés, agrandis d'un facteur entier : parfaitement nets, mais l'écran n'est pas rempli (×7 en Full HD, soit 1008 lignes sur 1080) |
| Net (`sharp`) | Remplit l'écran : pixels nets, dont seuls les bords sont adoucis sur un pixel de l'écran, pour éviter des pixels de tailles inégales |
| LCD (`lcd`) | La grille de l'écran d'origine : points séparés d'un fin interstice, et bandes rouge, verte et bleue sur Game Boy Color. Prend tout son sens en 4K (15 pixels de l'écran par pixel) ; s'estompe aux petites échelles |
| Scale2x, Scale3x | Arrondit les diagonales en gardant l'aspect pixel art, sans ajouter de couleurs |
| MMPX | Algorithme récent (McGuire et Gagiu, 2021) conçu pour le pixel art : lisse les pentes en préservant le texte, les contours et le tramage |

Les filtres à base d'algorithme agrandissent d'abord l'image d'un facteur fixe (×2 ou ×3) chaque
fois qu'elle change, puis la passe « Net » l'étend à l'écran : leur coût ne dépend pas de la
résolution de l'écran, et le jeu reste à 60 images par seconde en 4K.

L'entrée « Rémanence » de la page Affichage mélange chaque image à la précédente, comme
l'écran LCD de la console, lent à réagir : certains jeux faisaient clignoter des sprites d'une
image à l'autre pour les rendre transparents. Elle se combine à tous les filtres, et suit le
modèle de SameBoy :

- **Simple** : l'image et la précédente à parts égales.
- **Fidèle** : les poids alternent d'une ligne à l'autre (un tiers, deux tiers), et l'ordre
  s'inverse à chaque image, comme sur l'écran LCD. Un sprite qui clignote reste stable.

Les deux images mélangées sont toujours consécutives, même en avance rapide. Le mélange se
fait en lumière linéaire, pour que les sprites transparents ne paraissent pas trop sombres,
et après le filtre, qui voit des images nettes. Comme sur l'écran d'origine, les éléments en
mouvement laissent un léger halo.

## Son

Le son des quatre canaux est mixé en stéréo à 48 kHz, à bande limitée : les harmoniques des
ondes carrées ne se replient pas dans l'audible. La page Son du menu l'adapte à l'écoute, sans
toucher à l'émulation :

| Entrée | Valeur | Effet |
|---|---|---|
| Sortie | Stéréo (`stereo`, défaut) | Les deux côtés tels que la console les mixe |
| | Casque (`headphones`) | Chaque oreille entend un peu l'autre côté, adouci et légèrement en retard, comme avec des enceintes dans une pièce (crossfeed). Beaucoup de jeux jouent une voie d'un seul côté, ce qui fatigue vite au casque. Un son au centre ne change pas |
| | Mono (`mono`) | Les deux côtés mélangés, pour une seule enceinte |
| Filtre | Aucun (`off`, défaut) | Le son de la console, sans retouche |
| | Doux (`soft`) | Aigus un peu adoucis (passe-bas à 9 kHz) |
| | Chaud (`warm`) | Son plus rond, aigus nettement adoucis (passe-bas à 4,5 kHz) |
| | Haut-parleur (`speaker`) | Le petit haut-parleur de la console : mono, sans graves ni aigus (de 350 Hz à 5 kHz). L'entrée Sortie est alors grisée |
| Bruitages | Oui (défaut) ou non | Les sons des menus, synthétisés comme sur la console (ondes carrées, enveloppe de volume) : déplacement, changement d'un réglage, entrée dans une page ou choix, retour, et refus (entrée grisée, réglage au bout). L'écran de dépôt les joue aussi |

Les options `-stereo` et `-audio-filter` remplacent ces réglages au lancement. En mode
headless, `-wav` enregistre le son brut, ou avec ces options si elles sont données.

## Jouer sur la TV

On joue sur l'ordinateur, de préférence à la manette, et l'image et le son s'affichent sur la
TV.

**Sans câble, avec une Apple TV (Mac uniquement)** : macOS fait de l'Apple TV un second
écran, et gbe s'y affiche comme sur n'importe quel écran. Le retard reste assez faible pour
jouer.

1. Centre de contrôle > Recopie de l'écran > choisir l'Apple TV, en mode « Utiliser comme
   écran séparé ». Le son part vers l'Apple TV.
2. Dans gbe, ouvrir le menu (Échap, ou Start+Select à la manette), puis la page Affichage :
   - **Ecran** choisit l'écran où s'affiche gbe, ici l'Apple TV. L'entrée n'apparaît
     qu'avec au moins deux écrans, et la liste se met à jour quand on active la recopie
     après avoir lancé gbe ;
   - **Mode** passe en plein écran, comme F11, mais aussi à la manette.

gbe retient l'écran et le mode : au lancement suivant, il s'ouvre directement sur la TV, ou
sur l'écran principal si la TV n'est plus là. Le filtre LCD et l'échelle se règlent comme sur
un grand écran (voir [Affichage](#affichage-grands-écrans)).

**Avec un câble HDMI**, sur tous les systèmes : les mêmes entrées Ecran et Mode envoient gbe
sur la TV, sans retard ajouté.

gbe ne diffuse pas lui-même en AirPlay : la recopie d'écran d'AirPlay est chiffrée par un
procédé propre à Apple, et les autres modes d'AirPlay ont plusieurs secondes de retard, trop
pour jouer. La diffusion vers un Chromecast est à l'étude.

## Jeux DMG en couleurs

Comme une vraie Game Boy Color, gbe peut coloriser les jeux Game Boy. C'est une option :
par défaut, ils tournent sur la Game Boy d'origine. Pour l'activer, passer l'entrée
« Coloriser » de la page Affichage du menu à « oui » (elle apparaît pour les jeux Game Boy lancés sans `-model`)
puis « Réinitialiser », ou lancer le jeu avec `-model gbc` :

- **Palette automatique** : les jeux Nintendo reçoivent la palette que la boot ROM couleur
  prévoit pour leur titre (Tetris en jaune et rouge, Link's Awakening en rose…), les autres
  une palette par défaut.
- **12 palettes au choix** : celles que la console offrait en maintenant une direction, seule
  ou avec A ou B, pendant le logo. L'entrée Palette de la page Affichage et la touche P passent de « Auto »
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

### Vibrations

Les cartouches vibrantes (Pokémon Pinball, Perfect Dark, Top Gear Rally…) font vibrer la
manette. La force suit celle du moteur de la cartouche, que le jeu fait varier.

Les autres jeux peuvent aussi faire vibrer la manette : gbe devine les chocs d'après les sons
joués. Il suit chaque note des voies de bruit et de *sweep*, et commence par écarter la musique.
Celle-ci revient en rythme et rejoue sans cesse les mêmes instruments, souvent depuis une autre
routine du jeu que les bruitages. Parmi les bruitages, un bruit grave, fort et long (explosion,
impact, tonnerre) vibre fort. Une fréquence qui glisse vite vibre moins, encore moins si elle
monte (saut, bonus) que si elle descend (chute, coup). Une note répétée à chaque image ne fait
que bourdonner, et ne vibre pas. Chaque choc donne une secousse franche, ressentie même quand
le son est très bref, qui s'éteint avec lui.

L'entrée **Vibrations** de l'onglet Manette choisit quand la manette vibre (←/→ ou Entrée) :

| Valeur | Effet |
|---|---|
| non | Jamais |
| cartouches Rumble | Seulement avec les cartouches à moteur (par défaut) |
| tous les jeux | Aussi dans les autres jeux, d'après leurs sons |

L'entrée **Intensité** règle leur force avec un curseur (←/→), du plus doux au plus fort (par
défaut). Elle est grisée quand les vibrations sont coupées. Valider sur le curseur (A ou Entrée),
comme **Tester la vibration**, fait vibrer la manette une demi-seconde à cette intensité.

gbe ne peut pas toujours savoir si une manette vibre. Il le sait sous Windows : seules les
manettes XInput (Xbox et compatibles) y vibrent, et les entrées sont grisées pour les autres.
Sous macOS (manettes Xbox, PlayStation, MFi) et Linux (manettes à retour de force), le test
permet de vérifier.

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

### Régler les vibrations de tous les jeux

Le détecteur de chocs (`internal/gb/rumbledetect.go`) se règle sur de vraies parties :

- `gbe -rumble-debug` affiche en bas de l'écran la force de la vibration et la dernière note
  des voies 1 et 4 jugée par le détecteur : musique (`music`) ou bruitage (`fx`), et sa force.
- `gbe -rumble-record dossier` enregistre les moments joués, chacun avec l'état de départ, les
  boutons de chaque image et les repères posés avec **M** quand un choc devrait vibrer. Un
  moment se termine au rembobinage, au chargement d'un état, à la réinitialisation ou en
  quittant.
- `tools/rumblelab` rejoue un moment enregistré, ou un jeu depuis son save state avec des
  boutons scriptés. Il écrit une page avec les notes de chaque voie, la vibration devinée, les
  repères, le moteur des cartouches Rumble (référence : Pokémon Pinball), l'écran et le son :

```sh
go run ./tools/rumblelab -out /tmp/lab dossier/ZELDA-20261010-153000.json
open /tmp/lab/index.html
```

Il rejoue aussi la partie sans appuyer sur rien : les notes que les boutons ne changent pas
(la musique, les sons du décor) sont distinguées de celles qui répondent au joueur.

## Performances

Sur chaque pull request, la CI lance les benchmarks de la branche et ceux de sa branche de
base, en alternance sur la même machine. Le rapport des écarts est publié en commentaire de la
PR, mis à jour à chaque push : médianes, test statistique, 🟢 au-delà de 5 % plus rapide, ⚠️
au-delà de 10 % plus lent. Il est informatif : les machines de la CI sont partagées, donc
bruitées, et il ne fait jamais échouer la PR.

Le même workflow se lance aussi à la main, pour comparer deux versions quelconques : tags,
branches ou commits. Dans l'onglet Actions de GitHub, choisir « Benchmarks » puis
« Run workflow ». Ou en ligne de commande :

```sh
gh workflow run bench.yml -f base=v0.1.4 -f head=main
```

Les deux versions exécutent alors les mêmes benchmarks, ceux de [`bench/`](bench/bench_test.go),
avec la même ROM de benchmark. Ces benchmarks n'utilisent que l'API présente dans toutes les
versions, ce qui permet de mesurer aussi les releases plus anciennes que les benchmarks. Ce
qu'une version ne sait pas faire est ignoré : le mode Game Boy Color avant v0.1.3, les
snapshots avant v0.1.4. Le rapport est dans le résumé de l'exécution.

À chaque publication d'une release, le workflow compare automatiquement la nouvelle release
à la précédente (les brouillons sont ignorés), de la même façon. Le rapport est dans le
résumé de l'exécution « Benchmarks of release vX.Y.Z ».

La même comparaison en local, par exemple avec `main` :

```sh
git worktree add /tmp/gbe-main main
tools/benchcompare.sh /tmp/gbe-main . 10 /tmp/bench
go run ./tools/benchreport -base main -head branche /tmp/bench/base.txt /tmp/bench/head.txt
```

Avec `HARNESS=bench`, les deux versions exécutent les benchmarks de `bench/` :

```sh
git worktree add /tmp/gbe-v0.1.4 v0.1.4
HARNESS=bench tools/benchcompare.sh /tmp/gbe-v0.1.4 . 10 /tmp/bench
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
  - APU : 4 canaux, sortie à bande limitée (sans repliement des aigus), filtre passe-haut.
  - Timer, joypad, port série, MBC.
- `internal/ui` : le frontend Ebitengine.
  - Rendu avec la palette choisie, ou en couleurs (corrigées ou non) en mode Game Boy Color.
  - `internal/ui/scaler` : les filtres d'affichage, des chaînes de shaders Kage
    (`shaders/*.kage`). Ajouter un filtre : un shader, une entrée dans `scaler.Filters` et
    son nom `filter.<id>` dans les traductions.
  - Audio : tampon dont le taux d'échantillonnage s'ajuste légèrement pour compenser l'écart
    entre 60 Hz et 59,73 Hz.
  - Menu, configuration JSON.
- `internal/i18n` : les traductions de l'interface (fichiers JSON embarqués).
- `cmd/gbe` : le point d'entrée.
