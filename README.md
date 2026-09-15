<div align="center">

<a href="README.md"><img src="logo.png" alt="FlareTunnel" width="260"></a>

# FlareTunnel

**Proxy HTTP/HTTPS basé sur Cloudflare Workers pour la rotation d’egress, le tunnel CONNECT et le streaming longue durée.**

[![Go](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Cloudflare Workers](https://img.shields.io/badge/Cloudflare-Workers-F38020?logo=cloudflare&logoColor=white)](https://developers.cloudflare.com/workers/)
[![TLS](https://img.shields.io/badge/TLS-transport%20%2B%20MITM-2F80ED)](#tls-et-https-connect)
[![License](https://img.shields.io/badge/license-non--commercial-orange)](#licence-et-responsabilité)

**Français · [English](README.en.md)**

</div>

FlareTunnel est un proxy local et déployable qui route les requêtes à travers des Cloudflare Workers. Il fournit un point d’entrée HTTP proxy unique, répartit les requêtes entre plusieurs Workers, prend en charge l’authentification Basic et conserve le streaming des réponses SSE et LLM.

## Architecture

```mermaid
graph LR
    C[Client HTTP/HTTPS] -->|Proxy HTTP ou TLS transport| P[FlareTunnel]
    P -->|Rotation| R{Worker rotator}
    R --> W1[Cloudflare Worker 1]
    R --> W2[Cloudflare Worker 2]
    R --> W3[Cloudflare Worker N]
    W1 --> T[Site ou API cible]
    W2 --> T
    W3 --> T
```

Le chemin `CONNECT` est compatible avec les clients HTTPS. Lorsque l’interception TLS est activée, FlareTunnel établit un tunnel MITM explicite : le client fait confiance au CA public FlareTunnel, puis FlareTunnel crée une connexion HTTPS distincte vers le Worker. Le body upstream est relayé sans être interprété.

## Fonctionnalités principales

- **Rotation de Workers** en mode `random` ou `round-robin`.
- **Multi-comptes Cloudflare** avec distribution des créations selon les quotas configurés.
- **Proxy HTTP et HTTPS** avec prise en charge de `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`, `OPTIONS` et `CONNECT`.
- **Authentification Basic obligatoire** avant tout forwarding ou établissement de tunnel.
- **TLS de transport optionnel** sur le listener du proxy.
- **TLS MITM optionnel** pour l’interception de connexions HTTPS.
- **Streaming SSE et LLM** avec transfert progressif des chunks et sans timeout global du body.
- **Blacklists intégrées** pour réduire la consommation de requêtes Worker.
- **Statistiques et tests de connectivité** par Worker et par compte.
- **Export/import de configuration** pour les sauvegardes contrôlées.

## Pré-requis

- Go 1.22 ou version ultérieure.
- Un ou plusieurs comptes Cloudflare avec un token API autorisant la gestion des Workers.
- Un certificat public et une clé privée de CA MITM si l’interception HTTPS est utilisée.
- Un certificat serveur transport et sa clé si le listener doit accepter directement TLS.

## Installation et compilation

```bash
git clone https://github.com/johndoe237/FlareTunnel.git
cd FlareTunnel
go mod download
go build -ldflags="-s -w" -o flaretunnel .
```

Le script `build.sh` construit le binaire de la plateforme courante et propose une compilation croisée Linux, Windows et macOS.

```bash
./build.sh
```

## Configuration Cloudflare

La commande `config` configure les comptes Cloudflare de manière interactive :

```bash
./flaretunnel config
```

Le fichier de configuration local contient les comptes, tokens et identifiants nécessaires aux commandes de gestion. Traitez-le comme un secret : ne le committez pas et protégez ses permissions.

### Créer des Workers

```bash
./flaretunnel create --count 5
./flaretunnel create --count 10 --distribute
./flaretunnel create --count 3 --account main
```

`--distribute` répartit la création entre les comptes disponibles. `--account` limite l’opération à un compte précis.

### Lister et tester les Workers

```bash
./flaretunnel list
./flaretunnel list --verbose
./flaretunnel list --status
./flaretunnel test
./flaretunnel test --url https://httpbin.org/ip
./flaretunnel test --url https://example.com --method POST
```

`list --verbose` affiche les détails et l’état live. `list --status` se concentre sur les temps de réponse.

### Sauvegarder et restaurer

```bash
./flaretunnel export --output my_backup.json
./flaretunnel import --input my_backup.json
./flaretunnel import --input my_backup.json --merge
```

Examinez les sauvegardes avant de les transférer. Elles peuvent contenir des credentials Cloudflare.

### Supprimer des Workers

La suppression bornée est recommandée :

```bash
./flaretunnel cleanup --account main --count 20 --yes
```

Cette commande supprime au plus 20 Workers existants du compte `main`. Pour le comportement historique de suppression complète :

```bash
./flaretunnel cleanup --account main --yes
```

La forme bornée exige `--account`, `--count` et `--yes`. Elle ne demande pas de saisie interactive et ne supprime jamais plus que la limite indiquée.

## Démarrer le proxy

### Mode HTTP local

```bash
export AUTH_PROXY_BASIC="dXNlcjE6cGFzczE=" # Base64("user1:pass1")
./flaretunnel tunnel --verbose
```

Le proxy écoute par défaut sur `127.0.0.1:8080`. Configurez vos clients ainsi :

```text
HTTP proxy  : http://127.0.0.1:8080
HTTPS proxy : http://127.0.0.1:8080
```

Le client doit envoyer :

```http
Proxy-Authorization: Basic dXNlcjE6cGFzczE=
```

Une authentification absente ou incorrecte reçoit `407 Proxy Authentication Required` avec un challenge `Proxy-Authenticate: Basic`.

### Options du tunnel

```bash
./flaretunnel tunnel --verbose
./flaretunnel tunnel --workers 0,1,2 --mode random
./flaretunnel tunnel --port 9090 --blacklist blacklist.txt
./flaretunnel tunnel --upstream-proxy http://127.0.0.1:8080 --verbose
./flaretunnel tunnel --no-ssl-intercept
./flaretunnel tunnel --cache-certs
```

Options disponibles :

| Option | Description |
| --- | --- |
| `--host` | Adresse d’écoute. Défaut : `127.0.0.1`. |
| `--port` | Port d’écoute. Défaut : `8080`. |
| `--workers` | Liste d’indices Worker, par exemple `0,1,2`. |
| `--mode` | `random` ou `round-robin`. |
| `--blacklist` | Fichier de blacklist à utiliser. |
| `--upstream-proxy` | Proxy upstream facultatif. |
| `--upstream-verify-ssl` | Active la vérification TLS du proxy upstream. |
| `--cache-certs` | Conserve le cache de certificats MITM. |
| `--no-ssl-intercept` | Désactive l’interception TLS MITM. |
| `--block` | Active le blocage configuré. |
| `--unsafe` | Mode explicitement réservé aux environnements de test. |

## TLS et HTTPS CONNECT

### TLS MITM

Définissez les chemins du certificat public et de la clé privée du CA MITM :

```bash
export FLARETUNNEL_MITM_CA_CERT=/runtime/Flaretunnel-MITM-CA.crt
export FLARETUNNEL_MITM_CA_KEY=/runtime/Flaretunnel-MITM-CA.key
./flaretunnel tunnel --port 8080
```

Le client doit installer uniquement `Flaretunnel-MITM-CA.crt` dans son trust store. La clé privée ne doit jamais être distribuée au client.

Les certificats de domaine générés sont mis en cache uniquement lorsque `--cache-certs` est utilisé. La clé privée du CA doit rester protégée par des permissions `0600`.

### TLS de transport du listener

Pour protéger la connexion entre le client et le proxy, définissez :

```bash
export FLARETUNNEL_TRANSPORT_CERT=/runtime/Flaretunnel-Transport.crt
export FLARETUNNEL_TRANSPORT_KEY=/runtime/Flaretunnel-Transport.key
export FLARETUNNEL_TLS_SAN="proxy.example.com 203.0.113.42"
./flaretunnel tunnel --host 0.0.0.0 --port 8080
```

`FLARETUNNEL_TLS_SAN` accepte des noms DNS, des IPv4 et des IPv6 séparés par des espaces. `0.0.0.0` et `::` sont des adresses d’écoute et ne sont pas des SAN valides. Le certificat serveur doit contenir les identités utilisées par les clients.

Le certificat transport et sa clé sont des artefacts serveur. La clé privée du CA transport n’est pas nécessaire à FlareTunnel.

## Blacklists

Trois fichiers sont fournis :

| Fichier | Usage | Effet attendu |
| --- | --- | --- |
| `blacklist-minimal.txt` | Recommandé pour la navigation | Bloque analytics, images, polices et source maps. |
| `blacklist.txt` | Économie plus forte | Ajoute publicité, tracking, CSS/JS et CDN. Certaines pages peuvent être incomplètes. |
| `blacklist-aggressive.txt` | Automatisation ciblée | Conserve surtout HTML/API. Les navigateurs peuvent ne plus fonctionner correctement. |

La blacklist réduit le nombre de requêtes Worker mais peut modifier le rendu des sites. Testez le niveau choisi avec `test` et votre trafic réel.

## Streaming LLM et SSE

FlareTunnel ne modifie pas les payloads LLM et ne transforme pas `{"stream":true}`. Les réponses HTTP sont écrites au fil de leur réception ; le chemin `CONNECT` relaie directement les bytes sur la connexion TLS hijackée.

Le body n’a pas de timeout global. Les délais d’établissement de connexion et de réception des headers restent limités. Lorsque le client se déconnecte, le contexte upstream est annulé et le tunnel est fermé.

## Exemple Python

```python
import requests

proxy = "http://user1:pass1@127.0.0.1:8080"
proxies = {"http": proxy, "https": proxy}

response = requests.get(
    "https://httpbin.org/ip",
    proxies=proxies,
    timeout=30,
    verify=False,  # uniquement si le CA de test n’est pas installé
)
print(response.json()["origin"])
```

En production, installez le CA public approprié dans le trust store et laissez la vérification TLS activée.

## Référence CLI

```text
Usage: flaretunnel <command> [options]

Commands:
  config     Configure les credentials Cloudflare
  create     Crée des Workers
  list       Liste les Workers et les analytics
  test       Teste les Workers
  export     Exporte la configuration
  import     Importe la configuration
  cleanup    Supprime les Workers
  tunnel     Démarre le proxy local
```

## Tests de développement

```bash
gofmt -w *.go
go test ./...
go vet ./...
go build ./...
git diff --check
```

Les tests couvrent notamment l’authentification proxy, le tunnel `CONNECT`, le streaming SSE, les certificats TLS transport et la gestion bornée du cleanup.

## Sécurité et limites

FlareTunnel doit être utilisé uniquement avec des comptes, Workers et destinations pour lesquels vous disposez d’une autorisation. Respectez les conditions d’utilisation de Cloudflare et les lois applicables.

Ne désactivez pas la validation TLS en production. Ne publiez jamais les clés privées des CA, les fichiers de configuration contenant des tokens, ni les certificats runtime. Séparez toujours le CA MITM du CA transport.

## Licence et responsabilité

Consultez les fichiers de licence du dépôt et les conditions applicables aux dépendances. Ce projet est fourni à des fins d’administration, de test et de recherche. L’utilisateur est responsable de son usage.

## Références

- [Cloudflare Workers](https://developers.cloudflare.com/workers/ "Documentation Cloudflare Workers")
- [Go `crypto/tls`](https://pkg.go.dev/crypto/tls "Package TLS de Go")
- [Méthode HTTP CONNECT](https://developer.mozilla.org/en-US/docs/Web/HTTP/Methods/CONNECT "Documentation HTTP CONNECT")

---

[Lire cette documentation en anglais](README.en.md)
