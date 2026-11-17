# Audit et historique de configuration

Gately détient des clés DNS et des clés privées de certificats : il doit donc être possible de retrouver qui a fait quoi et quand, de suivre l'évolution de la configuration et d'annuler chaque changement.

## Journal d'audit

L'onglet « Journal d'audit » de la page « Système → Journaux », dans la barre latérale, liste les opérations suivantes de la plus récente à la plus ancienne. Vous pouvez filtrer par catégorie ou rechercher par chemin, utilisateur ou IP :

| Catégorie | Contenu enregistré |
| --- | --- |
| Connexion | Connexions réussies et échouées par mot de passe, passkey ou connexion via des tiers, ainsi que les déconnexions |
| Modification | Toutes les opérations d'écriture lancées depuis l'interface d'administration ou avec un jeton d'API (création, modification, suppression, démarrage/arrêt, restauration, etc.) |
| Accès sensible | Téléchargement de sauvegardes de configuration, de paquets de diagnostic ou de clés privées de certificats, ouverture du terminal Web ou d'un terminal de conteneur, etc. |

Chaque entrée contient l'heure, l'auteur (nom d'utilisateur ou `api-token:<nom du jeton>`), l'IP source, la méthode, le chemin et le code de statut. **Les corps de requêtes et les secrets ne sont jamais enregistrés**.

- Le journal est écrit dans `audit.log` (JSON Lines) dans le dossier de configuration ; au-delà de 5 MiB, le fichier est renouvelé et 3 fichiers précédents sont conservés ;
- Lorsque l'interface d'administration se trouve derrière un proxy inverse local, la source réelle est enregistrée et l'adresse du proxy traversé est indiquée à part ; le principe est décrit dans le [modèle de sécurité](/fr/docs/guide/security) ;
- Les modifications et les accès sensibles sont émis sous forme d'événements `audit.change` et `audit.access`, que le [centre de notifications](/fr/docs/modules/notify) peut envoyer avec les modèles « Modifications de configuration » et « Sécurité et connexions » ;
- Les comptes en lecture seule et les jetons d'API ne peuvent pas consulter le journal d'audit.

## Historique de configuration et restauration

Avant chaque écrasement d'un fichier de configuration, Gately enregistre l'ancien contenu comme une version d'historique dans `history/<nom>/` du dossier de configuration, et conserve les 30 plus récentes pour chaque configuration. `account.json` (identifiants du compte) et `tokens.json` (jetons) ne sont pas historisés.

Dans l'onglet « Historique de configuration » de la page « Système → Journaux » :

1. Choisissez un fichier de configuration pour voir la date d'enregistrement et la taille de chaque version ;
2. Cliquez sur « Comparer » pour afficher les différences ligne par ligne entre cette version et la configuration actuelle ;
3. Cliquez sur « Restaurer » pour rétablir cette version ; Gately redémarre ensuite automatiquement pour l'appliquer.

Le contenu actuel est lui aussi enregistré comme nouvelle version avant la restauration : la restauration elle-même peut donc être annulée.
