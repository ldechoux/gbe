# Traductions de l'interface

Ce dossier contient les libellés affichés par l'émulateur, une langue par fichier JSON dans
`locales/`. Les fichiers sont intégrés au binaire (`go:embed`) : aucune installation n'est
nécessaire, mais toute modification demande de recompiler.

## Ajouter une langue

1. Copier `locales/en.json` en `locales/<code>.json`, où `<code>` est le code ISO 639-1 de la
   langue en minuscules (`es`, `de`, `it`…). Le nom du fichier est la valeur enregistrée dans
   la clé `language` de la config.
2. Renseigner `name` avec le nom de la langue **dans cette langue** (`Espanol`, `Deutsch`) :
   c'est ce qu'affiche l'entrée « Language » du menu.
3. Traduire chaque valeur de `messages` sans toucher aux clés.
4. Lancer `go test ./internal/i18n`, puis recompiler. La langue apparaît automatiquement dans
   le menu, qui fait défiler les langues dans l'ordre alphabétique des codes.

Aucun code Go n'est à modifier.

### Format d'un fichier

```json
{
  "name": "Francais",
  "messages": {
    "menu.resume": "Reprendre",
    "menu.scale": "Echelle   < x%d >"
  }
}
```

### Règles

- **ASCII uniquement.** La police bitmap du menu n'a pas d'accents : écrire `Echelle`, pas
  `Échelle`. Un caractère hors ASCII serait dessiné avec une autre police.
- **Paramètres.** Certains messages contiennent des paramètres au format Go `fmt` (`%s`
  texte, `%d` entier, `%%` signe pourcent). La traduction doit garder les mêmes paramètres,
  dans le même ordre.
- **Alignement.** Les entrées du menu Pause de la forme `Libelle   < %s >` sont alignées
  en colonne : compléter avec des espaces pour que le `<` tombe en 11e position, comme en
  anglais. Les noms de boutons, les actions (`action.*`) et `controls.screenshot` sont
  affichés sur une colonne de 8 caractères : au-delà, l'alignement de la page Contrôles se
  décale.
- **Longueur.** Le panneau du menu s'élargit pour le libellé le plus long ; rester concis.
- **Messages manquants.** Un message absent d'une langue s'affiche en anglais, mais les tests
  exigent que chaque langue définisse exactement les mêmes clés que `en.json`.

### Ce que vérifie `go test ./internal/i18n`

- chaque fichier est un JSON valide avec un `name` ;
- chaque langue a exactement les clés de `en.json` (ni manquante, ni inconnue) ;
- chaque message a les mêmes paramètres `%` que sa version anglaise ;
- tous les textes sont en ASCII ;
- toutes les clés sont documentées dans ce fichier.

## Ajouter un libellé (pour les développeurs)

1. Ajouter la clé dans **tous** les fichiers de `locales/`, en commençant par `en.json`.
2. L'utiliser dans le code avec `g.tr().T("ma.cle")`, ou `l.T("ma.cle", args...)` quand une
   `*i18n.Locale` est déjà disponible.
3. La documenter dans le tableau ci-dessous.

Les clés sont préfixées par la zone de l'interface où elles apparaissent.

## Liste des clés

### `menu.*` — menu Pause (Échap, ou Start+Select à la manette)

| Clé | Paramètres | Usage |
|---|---|---|
| `menu.title` | | Titre du menu Pause |
| `menu.resume` | | Entrée qui ferme le menu et reprend le jeu |
| `menu.palette` | `%s` nom de la palette | Entrée de choix de la palette (pour un jeu DMG colorisé : `colors.auto` ou une combinaison de boutons, comme `Gauche+B`) |
| `menu.colors` | `%s` `colors.corrected` ou `colors.raw` | Remplace `menu.palette` en mode Game Boy Color : active la correction des couleurs |
| `colors.corrected` | | Couleurs ajustées pour ressembler à l'écran de la Game Boy Color |
| `colors.raw` | | Couleurs telles que le jeu les définit, sans correction |
| `colors.auto` | | Palette qu'une Game Boy Color choisit d'après le titre d'un jeu DMG |
| `menu.colorize` | `%s` `setting.on` ou `setting.off` | Entrée qui active la colorisation des jeux DMG (appliquée par Réinitialiser), absente pour les jeux Game Boy Color et avec `-model` |
| `setting.on` | | Réglage activé |
| `setting.off` | | Réglage désactivé |
| `menu.controls` | | Entrée qui ouvre la page Contrôles |
| `menu.volume` | `%d` volume en pourcent | Entrée de réglage du volume (`%%` affiche `%`) |
| `menu.scale` | `%d` facteur d'échelle | Entrée de réglage de la taille de la fenêtre |
| `menu.fast_forward` | `%d` vitesse (2 à 8) | Entrée de réglage de la vitesse de l'avance rapide |
| `menu.language` | `%s` valeur `name` de la langue | Entrée de choix de la langue |
| `menu.save_state` | | Entrée qui sauvegarde l'état de la partie |
| `menu.load_state` | | Entrée qui recharge l'état sauvegardé |
| `menu.reset` | | Entrée qui réinitialise la console |
| `menu.quit` | | Entrée qui quitte l'émulateur |
| `menu.footer` | | Aide en bas du menu (raccourcis clavier) |
| `menu.footer_pad` | | Aide en bas du menu quand une manette est branchée |
| `menu.footer_color` | | `menu.footer` en mode Game Boy Color, où P règle la correction des couleurs |
| `menu.footer_pending` | | Remplace l'aide en bas du menu tant que le mode de la console (colorisé ou non) diffère du réglage : Réinitialiser l'applique |
| `menu.footer_color_pad` | | `menu.footer_pad` en mode Game Boy Color |

### `start.*` — écran de reprise (au lancement, si une sauvegarde d'état existe)

| Clé | Paramètres | Usage |
|---|---|---|
| `start.title` | | Titre de l'écran |
| `start.resume` | | Entrée qui recharge la partie sauvegardée |
| `start.fresh` | | Entrée qui démarre une nouvelle partie |
| `start.footer` | | Aide en bas de l'écran, quand la date de sauvegarde est inconnue |
| `start.saved_at` | `%s` date formatée | Bas de l'écran : date de la sauvegarde |
| `start.other_mode_dmg` | | Ligne ajoutée sous la date quand la partie a été sauvegardée en DMG alors que le réglage demande la colorisation |
| `start.other_mode_color` | | Même chose pour une partie colorisée quand la colorisation est désactivée |
| `start.date_format` | | Format de cette date, au format Go `time` (voir ci-dessous) |

`start.date_format` n'est pas un texte libre mais un modèle de date Go : `02` jour,
`01` mois, `2006` année, `15` heure, `04` minutes. Exemple : `02/01/2006 a 15:04` donne
`27/09/2026 a 11:30`. Le reste du texte (`a`, `at`) est recopié tel quel, à condition de ne
contenir aucun de ces nombres ni de mots comme `Jan` ou `Mon`, que Go interpréterait.

### `controls.*` — page Contrôles

| Clé | Paramètres | Usage |
|---|---|---|
| `controls.title` | | Titre de la page |
| `controls.keyboard` | | Onglet Clavier |
| `controls.gamepad` | | Onglet Manette |
| `controls.screenshot` | | Ligne du raccourci de capture d'écran (8 caractères max) |
| `controls.press_key` | | Remplace la touche pendant qu'on attend la nouvelle touche |
| `controls.press_button` | | Remplace le bouton pendant qu'on attend le nouveau bouton de manette |
| `controls.press_combo` | | Remplace le raccourci pendant qu'on attend la nouvelle combinaison |
| `controls.defaults` | | Entrée qui rétablit les réglages par défaut de l'onglet |
| `controls.back` | | Entrée qui revient au menu Pause |
| `controls.footer` | | Aide en bas de l'onglet Clavier |
| `controls.cancel` | | Aide en bas de la page pendant une attente de touche ou de bouton |
| `controls.pad_name` | `%s` nom de la manette | Bas de l'onglet Manette |
| `controls.pad_disconnected` | | Message quand la manette est débranchée sur l'onglet Manette |
| `controls.vibration` | | Entrée de l'onglet Manette qui active les vibrations, suivie de oui/non |
| `controls.vibration_test` | | Entrée de l'onglet Manette qui fait vibrer la manette une demi-seconde |
| `controls.unavailable` | | Remplace oui/non après `controls.vibration` quand la manette ne peut pas vibrer |
| `controls.no_vibration` | | Bas de l'onglet Manette sur les entrées de vibration grisées |
| `controls.key_used_by_button` | `%s` nom de la touche | Refus : la touche choisie pour la capture sert déjà à un bouton |
| `controls.key_used_by_screenshot` | `%s` nom de la touche | Refus : la touche choisie pour un bouton sert déjà à la capture |

### `button.*` — boutons de la Game Boy (page Contrôles, 8 caractères max)

| Clé | Usage |
|---|---|
| `button.up` | Croix directionnelle, haut |
| `button.down` | Croix directionnelle, bas |
| `button.left` | Croix directionnelle, gauche |
| `button.right` | Croix directionnelle, droite |
| `button.a` | Bouton A |
| `button.b` | Bouton B |
| `button.start` | Bouton Start |
| `button.select` | Bouton Select |

### `action.*` — actions de l'émulateur (page Contrôles, 8 caractères max)

| Clé | Usage |
|---|---|
| `action.fast_forward` | Touche à maintenir pour l'avance rapide |
| `action.rewind` | Touche à maintenir pour revenir en arrière |

### `toast.*` — notifications temporaires en bas à gauche de l'écran

| Clé | Paramètres | Usage |
|---|---|---|
| `toast.state_saved` | | L'état de la partie a été sauvegardé |
| `toast.save_failed` | `%s` erreur | La sauvegarde de l'état a échoué |
| `toast.no_state` | | Chargement demandé sans sauvegarde existante |
| `toast.state_unreadable` | `%s` erreur | La sauvegarde existe mais n'a pas pu être chargée |
| `toast.state_restored` | | L'état a été rechargé |
| `toast.screenshot` | `%s` nom du fichier | Capture d'écran enregistrée |
| `toast.screenshot_failed` | `%s` erreur | La capture d'écran a échoué |
| `toast.restart_failed` | `%s` erreur | Le redémarrage dans le mode choisi a échoué |

### `title.*` — titre de la fenêtre

| Clé | Usage |
|---|---|
| `title.paused` | Remplace le nombre d'images par seconde quand le jeu est en pause |

### `key.*` — noms des touches du clavier

Les touches qui produisent un caractère (lettres, chiffres, ponctuation) affichent ce
caractère selon la disposition du clavier, sans traduction. Ces clés nomment les autres.

| Clé | Paramètres | Usage |
|---|---|---|
| `key.arrow_up` | | Flèche haut |
| `key.arrow_down` | | Flèche bas |
| `key.arrow_left` | | Flèche gauche |
| `key.arrow_right` | | Flèche droite |
| `key.enter` | | Entrée |
| `key.numpad_enter` | | Entrée du pavé numérique |
| `key.space` | | Barre d'espace |
| `key.tab` | | Tabulation |
| `key.backspace` | | Retour arrière |
| `key.escape` | | Échap |
| `key.delete` | | Suppr |
| `key.insert` | | Inser |
| `key.home` | | Début |
| `key.end` | | Fin |
| `key.page_up` | | Page précédente |
| `key.page_down` | | Page suivante |
| `key.caps_lock` | | Verrouillage majuscules |
| `key.shift_left` | | Maj gauche |
| `key.shift_right` | | Maj droite |
| `key.control_left` | | Ctrl gauche |
| `key.control_right` | | Ctrl droite |
| `key.alt_left` | | Alt gauche |
| `key.alt_right` | | Alt droite |
| `key.meta_left` | `%s` `Cmd` (macOS) ou `Meta` | Touche Cmd / Windows gauche |
| `key.meta_right` | `%s` `Cmd` (macOS) ou `Meta` | Touche Cmd / Windows droite |
| `key.numpad` | `%s` touche du pavé (`1`, `Add`…) | Touche du pavé numérique, pour la distinguer de la rangée principale |

### `pad.*` — noms des boutons de manette

Seuls les noms qui varient selon la langue sont traduits ; les sigles imprimés sur les
manettes (`A`, `LB`, `R2`, `Options`, `Home`…) restent tels quels.

| Clé | Usage |
|---|---|
| `pad.view` | Bouton Vue des manettes Xbox |
| `pad.cross` | Bouton Croix des manettes PlayStation |
| `pad.circle` | Bouton Rond des manettes PlayStation |
| `pad.square` | Bouton Carré des manettes PlayStation |
| `pad.triangle` | Bouton Triangle des manettes PlayStation |
| `pad.dpad_up` | Croix directionnelle de la manette, haut |
| `pad.dpad_down` | Croix directionnelle de la manette, bas |
| `pad.dpad_left` | Croix directionnelle de la manette, gauche |
| `pad.dpad_right` | Croix directionnelle de la manette, droite |
| `pad.left_stick` | Clic du stick gauche |
| `pad.right_stick` | Clic du stick droit |
