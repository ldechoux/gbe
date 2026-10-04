---
name: site-credits
description: Met à jour la section « Crédits » du site du projet (site/index.html.tmpl, #credits), qui remercie les projets externes sur lesquels gbe s'appuie, avec un lien vers chacun. Repère les bibliothèques, inspirations, ROMs de test et outils ajoutés ou retirés depuis la dernière mise à jour, rédige les cartes selon les règles de la section, vérifie les liens et montre l'aperçu avant toute PR. À utiliser quand l'utilisateur demande de mettre à jour, compléter ou revoir les crédits du site, ou après l'ajout d'une dépendance, d'un algorithme ou d'une source d'inspiration.
---

# Mettre à jour les crédits du site

La section « Crédits » (`site/index.html.tmpl`, `<section id="credits">`, lien « Crédits »
dans la barre de navigation) remercie les projets dont gbe dépend ou s'inspire. Elle est en
français, comme tout le site.

## 1. Faire l'inventaire

```sh
git checkout main && git pull --ff-only
git checkout -b feature/site-credits-<sujet>
.claude/skills/site-credits/candidates.sh
```

Le script ne modifie rien. Il affiche :
- les cartes actuelles, par groupe ;
- les dépendances directes de `go.mod`, et les modules réellement importés par
  `cmd/gbe` ;
- les projets et les URL cités dans le code et la documentation ;
- le code HTTP de chaque lien de la section.

Comparer avec les cartes actuelles. Pour chaque candidat, lire le code ou la doc qui le
cite (`git grep -n <nom>`) pour savoir ce que gbe lui doit vraiment.

## 2. Décider quoi créditer

**On crédite**, en trois groupes, dans cet ordre :

| Groupe | Ce qui y va | Exemples actuels |
|---|---|---|
| Bibliothèques | Le langage, et les bibliothèques qui font tourner gbe chez le joueur | Ebitengine (avec Oto et purego), Go |
| Inspirations | Les projets dont gbe reprend des tables, des algorithmes, des formules, un comportement ou un format | SameBoy, MMPX et Scale2x/Scale3x, higan, BGB |
| Tests et outils | Les ROMs de test et les outils qui valident ou construisent gbe | Tests de blargg, dmg-acid2 et cgb-acid2, RGBDS |

**On ne crédite pas** :
- **les dépendances d'outillage de développement**, sans effet pour le joueur : par
  exemple `golang.org/x/perf`, qui ne sert qu'aux rapports de performances ;
- **les modules de Go** (`golang.org/x/...`) : la carte Go ne parle que du langage, sans
  citer ses modules ni les polices du menu ;
- **les dépendances indirectes**, couvertes par la carte de leur bibliothèque : `rasterx`,
  `go-text/typesetting` et les autres modules `ebitengine/*` relèvent d'Ebitengine. Oto et
  purego y sont nommés parce qu'ils comptent pour le joueur (le son, et le Go sans cgo) ;
- **un projet dont la fonctionnalité a été retirée** : par exemple xBR, abandonné.

**Les cas douteux sont à soumettre à l'utilisateur** avant de les ajouter : un projet cité
pour un détail, comme un nom de palette ou un format de fichier repris. BGB était un tel
cas : l'utilisateur a choisi de le créditer pour sa palette et le format de l'horloge MBC3.
Ne rien ajouter ni retirer d'office.

## 3. Écrire les cartes

Chaque carte suit ce modèle, dans le `<div class="grid features credits">` de son groupe :

```html
<article class="card">
  <h3><a target="_blank" rel="noopener" href="https://projet.example">Nom du projet</a></h3>
  <p>Ce que c'est, et son auteur. Ce que gbe lui doit, concrètement. <a target="_blank" rel="noopener" href="https://depot.example">Code source</a>.</p>
</article>
```

- **Le titre est un lien** vers la page officielle du projet : son site s'il en a un,
  sinon son dépôt. Une carte qui regroupe plusieurs projets, comme « Algorithmes des
  filtres », a un titre sans lien et met les liens dans le texte. Le lien « Code source »
  n'est utile que si le titre pointe vers un site.
- **L'auteur** est nommé quand il est connu : « de Lior Halphon », « de Matt Currie ».
- **Ce que gbe doit au projet** : une ou deux phrases précises et vérifiées, qui parlent
  de ce que voit ou fait le joueur (« pour coloriser les jeux Game Boy ») plutôt que de
  l'implémentation.
- **Ne rien affirmer sans l'avoir vérifié.** Par exemple, les ROMs de blargg et acid2
  servent aux tests en local, pas sur la CI : on écrit « que gbe réussit », pas « à chaque
  modification ».
- **La longueur** reste comparable à celle des autres cartes, de 2 à 4 lignes à l'écran.
  La grille a 3 colonnes : un groupe de 1 ou 2 cartes laisse des cases vides, ce qui est
  accepté.
- Le style existe déjà (`.credits h3 a` dans `site/style.css`) : n'ajouter du CSS que si
  la structure change.

**Les liens** :
- Comme tous les liens du site qui le quittent, chaque lien s'ouvre dans un nouvel
  onglet, avec `target="_blank" rel="noopener"`. Seuls les ancres de la page (`#…`) et les
  boutons de téléchargement n'en ont pas.
- `candidates.sh` les vérifie : chacun doit répondre 200 et avoir ces attributs.
- Une page qui se charge en JavaScript ne montre pas son contenu à curl ni à WebFetch :
  confirmer alors par une recherche web que l'URL est bien la bonne, par exemple la page
  des auteurs de MMPX.
- Si le site d'un projet a disparu, pointer vers son dépôt actuel, comme higan.

## 4. Montrer l'aperçu

```sh
gh api -H "Accept: application/vnd.github.html+json" repos/ldechoux/gbe/releases > <scratchpad>/releases.json
go run ./tools/sitegen -releases <scratchpad>/releases.json -out <scratchpad>/www
go test ./tools/sitegen
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --headless=new --disable-gpu \
  --hide-scrollbars --force-device-scale-factor=1 --window-size=1280,16000 \
  --virtual-time-budget=5000 --screenshot=<scratchpad>/page.png "file://<scratchpad>/www/index.html"
magick <scratchpad>/page.png -trim +repage <scratchpad>/page-trim.png
```

- Le rendu headless ne suit pas l'ancre `#credits`. La section est juste avant le pied de
  page : recadrer le bas de `page-trim.png` (environ 1 300 px) et le vérifier : cartes
  alignées, liens visibles, texte lisible.
- Ouvrir `<scratchpad>/www/index.html` dans le navigateur de l'utilisateur avec `open`.

## 5. Attendre la validation, puis faire la PR

Présenter les cartes ajoutées, modifiées ou retirées, avec la raison de chaque changement
et les cas douteux à trancher. Appliquer ses corrections, puis remontrer l'aperçu.

Ne commiter, pousser et ouvrir la PR qu'après son accord explicite. Le commit et la PR
sont en anglais, comme le reste de l'historique. La PR liste les projets crédités par
groupe et le plan de test (liens vérifiés, `go test ./tools/sitegen`, aperçu validé). Après
le merge, le workflow GitHub Pages redéploie le site, puisque `site/**` le déclenche.
