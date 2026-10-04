---
name: release
description: Prépare une nouvelle version de gbe sur GitHub, en brouillon (draft). Rédige la note de version, en français, à partir des pull requests mergées depuis la dernière release, dans le style des notes précédentes. Numéro semver : celui passé en argument (par exemple « /release 0.2.0 »), sinon la version corrective suivante de la dernière release (v0.1.6 → v0.1.7). À utiliser quand l'utilisateur demande de créer, préparer ou rédiger une release, une version ou une note de version.
---

# Préparer une release

Le skill crée la release **en brouillon** et ne la publie jamais : c'est l'utilisateur qui
publie. La publication crée le tag, puis déclenche la construction des binaires
(`release.yml`), les benchmarks contre la version précédente (`bench.yml`) et le
redéploiement du site (`pages.yml`).

## 1. Version et changements

```sh
.claude/skills/release/context.sh [version]
```

- **Avec un numéro** (`0.2.0`, `v0.2.0`, `v1.0.0-rc.1`…), le skill respecte ce numéro.
  Le script le normalise avec un `v` en tête, vérifie qu'il est semver et postérieur à la
  dernière release, et que le tag n'existe pas encore. Une version avec suffixe
  (`-rc.1`…) est une pré-version : la release sera marquée `--prerelease`.
- **Sans numéro**, le script incrémente le numéro de correctif de la dernière release
  publiée (v0.1.6 → v0.1.7).
- **En cas d'erreur** (numéro invalide, pas plus récent, tag existant), s'arrêter et la
  rapporter à l'utilisateur, sans choisir un autre numéro à sa place.
- **Si `existing` vaut `draft`**, un brouillon existe déjà pour cette version. Le mettre à
  jour (étape 5) plutôt qu'en créer un second. Si `existing` vaut `published`, s'arrêter.

Le script affiche aussi :
- les pull requests mergées depuis la dernière release, avec un commit de merge ou en
  squash (le titre du commit se termine alors par `(#N)`) ;
- les commits poussés sur `main` hors pull request ;
- le format des save states (`stateVersion` dans `internal/gb/state.go`) ;
- les fichiers modifiés.

S'il n'y a aucun changement, le dire et ne rien créer.

## 2. Lire les sources

- **Le style** : lire en entier les deux ou trois dernières notes.

  ```sh
  gh release view <tag> --json body -q .body
  ```
- **Chaque pull request**, avec sa description :

  ```sh
  gh pr view <n> --json title,body
  ```

  Au besoin, lire aussi le diff ou le code, pour décrire le comportement réel et non
  l'intention.
- **Les commits hors pull request**, avec `git show`.

## 3. Rédiger la note

Écrire la note dans un fichier du scratchpad, en Markdown, en français.

**Structure**, celle des notes précédentes :

1. **Résumé** : une ou deux phrases qui annoncent les nouveautés principales en gras, par
   exemple « Cette version apporte des **filtres d'affichage pour jouer sur grand
   écran**… ».
2. **`## Nouveautés`** : une sous-section `###` par thème, du plus visible au moins
   visible.
   - Chaque thème s'ouvre éventuellement sur un paragraphe de contexte, puis liste des
     puces « **Libellé** : phrase. ».
   - Un tableau peut présenter des options parallèles, comme les filtres de la v0.1.6.
   - Thèmes habituels quand ils s'appliquent :
     - `### Corrections` : les bugs corrigés, avec le symptôme vu par le joueur ;
     - `### Site du projet` : ce que présente maintenant https://ldechoux.github.io/gbe/.
3. **`## Fiabilité`**, seulement si c'est notable : tests, ROMs de test, suivi des
   performances.
4. **`## Mise à jour`** :
   - « Remplace simplement le binaire. Tes réglages, les sauvegardes (`.sav`) et les save
     states (`.state`) des versions précédentes sont conservés. » Adapter si un réglage
     ou un format change.
   - Une phrase sur les save states, d'après `context.sh` :
     - inchangé : « Le format des save states ne change pas : ceux créés avec la vX se
       chargent aussi dans la vY. » ;
     - changé : « Le format des save states évolue : ceux créés avec la vX ne se chargent
       pas dans une version précédente. »
5. **La dernière ligne** :
   `**Changements complets** : https://github.com/ldechoux/gbe/compare/<dernière>...<nouvelle>`

**Ton** :
- S'adresser au joueur, en le tutoyant (« maintiens Tab », « tes réglages »). Décrire ce
  qu'il voit et fait, pas l'implémentation.
- Nommer les touches, les entrées du menu et les options telles qu'elles s'affichent :
  « l'entrée « Filtre » de la page Affichage », `-filter`, **F11**.
- Des phrases courtes et précises, avec des exemples concrets (noms de jeux, valeurs).

**Ce qu'on ne met pas** :
- l'outillage de développement sans effet pour le joueur : skills `.claude/`, refactors
  internes, CI. On le met en « Fiabilité » seulement s'il protège la qualité de façon
  visible, comme les tests ou le suivi des performances ;
- une fonctionnalité ajoutée puis retirée pendant le cycle, qui n'existe pas dans la
  version ;
- un chiffre non vérifié. Les performances viennent de mesures (rapport de benchmarks
  d'une pull request, mesure faite pendant le cycle), avec la machine si elle compte :
  « sur un Mac M1 Max ».

Les fonctionnalités expérimentales sont présentées comme telles, avec la même formulation
que le README.

**Sur le site** : la section « Notes de version » de https://ldechoux.github.io/gbe/
affiche en détail les notes des **trois dernières releases publiées**. Les précédentes n'y
sont qu'un lien vers leur page GitHub. `tools/sitegen` applique cette règle à chaque
redéploiement (constante `detailedReleases`), donc rien n'est à modifier à la main. Une
note doit se suffire à elle-même : ne pas renvoyer à une note plus ancienne, qui ne sera
plus détaillée sur le site. Si l'utilisateur change le nombre de notes détaillées, modifier
`detailedReleases` et son test dans `tools/sitegen`, puis ce paragraphe.

## 4. Relire

Avant de créer le brouillon :
- relire chaque affirmation de la note par rapport aux pull requests et au code ;
- vérifier qu'aucun changement visible pour le joueur ne manque ;
- vérifier le lien de comparaison et les numéros de version ;
- vérifier les noms affichés : comparer les entrées et les touches avec
  `internal/i18n/locales/fr.json`.

## 5. Créer le brouillon

Cibler le commit exact de `main` affiché par `context.sh` (`target`), et non la branche :
la note décrit ce commit, même si `main` avance avant la publication.

```sh
gh release create <version> --draft --target <sha> --title "gbe <version>" --notes-file <note.md> [--prerelease]
```

Si un brouillon existe déjà pour cette version :

```sh
gh release edit <version> --notes-file <note.md> --target <sha>
```

Puis vérifier :

```sh
gh release view <version> --json isDraft,name,targetCommitish,url
```

## 6. Rendre compte

Donner à l'utilisateur :
- le lien du brouillon. L'URL est en `untagged-…` tant que la release n'est pas publiée ;
- la version et pourquoi : celle demandée, ou la version corrective suivante ;
- ce que couvre la note, pull request par pull request, et ce qui a été laissé de côté,
  avec la raison ;
- les points à savoir :
  - le tag est créé à la publication, sur le commit ciblé ;
  - les binaires n'apparaissent qu'après la publication ;
  - les benchmarks et le redéploiement du site se lancent au même moment. Sur le site, la
    nouvelle note devient l'une des trois notes détaillées, et la plus ancienne des trois
    passe dans la liste des versions précédentes.

Ne jamais publier la release, même si l'utilisateur semble pressé : il publie lui-même
depuis GitHub. Après la publication, s'il le demande, suivre les workflows
`Release binaries`, `GitHub Pages` et `Benchmarks` (`gh run watch`), puis résumer le
rapport de performances, qui est dans l'artefact `benchmarks` du run.
