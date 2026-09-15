# FlareTunnel

[English version](README.en.md)

FlareTunnel est un proxy HTTP/HTTPS qui achemine les requêtes vers des Cloudflare Workers. Il prend en charge la rotation de Workers, plusieurs comptes Cloudflare, l’authentification du proxy, le passthrough HTTP, le tunnel `CONNECT`, l’interception TLS MITM optionnelle et les réponses SSE longues.

> FlareTunnel est un composant de proxy. Le déploiement recommandé en production est effectué par `FlareTunnel-Manager`, qui fournit le binaire, les certificats et les secrets au runtime.

## Architecture

```text
Client / OmniRoute
        │
        │ HTTP proxy ou HTTPS proxy transport
        ▼
FlareTunnel
        │  CONNECT + TLS MITM optionnel
        ▼
Cloudflare Worker
        │
        ▼
Fournisseur ou site cible
```

Le TLS de transport et le TLS MITM sont deux couches distinctes. Le premier protège la connexion entre le client et le listener proxy. Le second présente au client un certificat temporaire pour le domaine cible, signé par le CA MITM. FlareTunnel établit ensuite sa propre connexion HTTPS vers le Worker.

## Fonctionnalités

- Rotation des Workers en mode aléatoire ou round-robin.
- Gestion de plusieurs comptes Cloudflare.
- Proxy HTTP classique et tunnel HTTPS avec `CONNECT`.
- Authentification obligatoire par `Proxy-Authorization: Basic`.
- TLS de transport optionnel directement dans FlareTunnel.
- TLS MITM optionnel pour les requêtes HTTPS.
- Relais progressif des corps SSE et des réponses LLM longues.
- Délais limités à l’établissement de connexion et aux headers upstream ; aucun délai global du body SSE.
- Listes de blocage minimale, complète ou agressive.

## Installation locale

```bash
git clone https://github.com/johndoe237/FlareTunnel.git
cd FlareTunnel
go build -o flaretunnel .
```

Go 1.21 ou une version ultérieure est recommandé.

## Configuration du proxy

L’authentification du proxy est obligatoire. La variable `AUTH_PROXY_BASIC` contient uniquement le résultat Base64 de `username:password`.

```bash
export AUTH_PROXY_BASIC="dXNlcjE6cGFzczE="
```

Cette valeur correspond à `user1:pass1`. Elle ne doit pas être confondue avec un mot de passe en clair.

### Mode HTTP local

Le mode HTTP local est activé lorsque les variables TLS de transport ne sont pas définies.

```bash
./flaretunnel tunnel \
  --host 127.0.0.1 \
  --port 8080 \
  --mode round-robin \
  --blacklist blacklist-minimal.txt
```

### TLS MITM pour `CONNECT`

Pour intercepter les connexions HTTPS, FlareTunnel reçoit le certificat public et la clé privée du CA MITM par chemins de fichiers. La clé privée ne doit jamais être ajoutée au dépôt ni à l’image cliente.

```bash
export FLARETUNNEL_MITM_CA_CERT=/runtime/Flaretunnel-MITM-CA.crt
export FLARETUNNEL_MITM_CA_KEY=/runtime/Flaretunnel-MITM-CA.key
./flaretunnel tunnel --port 8080
```

Le client doit faire confiance au certificat public `Flaretunnel-MITM-CA.crt`.

### TLS de transport du listener

Le listener peut lui-même accepter une connexion TLS lorsque ces variables sont toutes définies :

```bash
export FLARETUNNEL_TRANSPORT_CERT=/runtime/Flaretunnel-Transport.crt
export FLARETUNNEL_TRANSPORT_KEY=/runtime/Flaretunnel-Transport.key
export FLARETUNNEL_TLS_SAN="proxy.example.com 203.0.113.42"
./flaretunnel tunnel --port 8080
```

`FLARETUNNEL_TLS_SAN` accepte des noms DNS, des adresses IPv4 et des adresses IPv6 séparés par des espaces. `0.0.0.0` et `::` sont des adresses d’écoute, pas des identités de certificat, et sont refusées. Le certificat serveur doit être signé par le CA transport et contenir les SAN configurés.

La clé privée du CA transport n’est pas utilisée par FlareTunnel. Elle reste dans le manager qui génère le certificat serveur éphémère.

## Commandes

| Commande | Fonction |
| --- | --- |
| `config` | Configure les comptes Cloudflare. |
| `create` | Crée des Workers proxy. |
| `list` | Liste les Workers disponibles. |
| `test` | Teste la connectivité des Workers. |
| `tunnel` | Démarre le proxy local. |
| `export` | Exporte la configuration. |
| `import` | Importe une configuration. |
| `cleanup` | Supprime des Workers, avec une forme bornée recommandée. |

Exemples :

```bash
./flaretunnel config
./flaretunnel create --count 5 --account main
./flaretunnel list --verbose
./flaretunnel test --url https://example.com
./flaretunnel cleanup --account main --count 5 --yes
```

La forme bornée de `cleanup` exige `--account`, `--count` et `--yes`. Elle ne supprime jamais plus de Workers que la limite demandée.

## Utilisation comme proxy

Pour un client HTTP classique :

```text
HTTP proxy  : http://proxy-user:password@127.0.0.1:8080
HTTPS proxy : http://proxy-user:password@127.0.0.1:8080
```

Pour un listener TLS de transport :

```text
HTTPS proxy : https://proxy-user:password@proxy.example.com:8080
```

Le certificat public du CA transport doit être installé dans le trust store du client. Le CA MITM est requis séparément pour valider les certificats de domaines interceptés.

## Streaming et passthrough

FlareTunnel ne parse pas les bodies LLM et ne transforme pas `{"stream":true}`. Le chemin HTTP écrit chaque bloc reçu et appelle `Flush` lorsque le serveur le permet. Le chemin `CONNECT` écrit directement sur la connexion TLS hijackée et recrée uniquement le framing HTTP nécessaire.

Aucun timeout global ne coupe le body SSE. La déconnexion du client annule le contexte de la requête upstream et ferme le tunnel.

## Déploiement recommandé

Pour un déploiement PaaS, VPS ou local avec la même image Docker, utilisez `FlareTunnel-Manager`. Le manager construit son image avec le binaire FlareTunnel, embarque uniquement les certificats publics, reçoit les clés privées par secret, génère le certificat transport et lance FlareTunnel comme child process.

`omni-boot` est un déploiement distinct. Il ne partage pas l’image du manager et communique avec FlareTunnel uniquement par le protocole proxy et les certificats publics nécessaires à la validation TLS.

## Sécurité

Ne désactivez jamais la validation TLS côté client. N’utilisez pas `rejectUnauthorized: false` ni `NODE_TLS_REJECT_UNAUTHORIZED=0`. Ne publiez aucune clé privée, valeur Base64 de clé, certificat serveur éphémère ou artefact runtime.

Les CA MITM et transport sont indépendants. Ne les remplacez pas et ne mélangez pas leurs clés privées. Une rotation d’un CA nécessite une opération coordonnée avec les clients qui lui font confiance.

## Tests de développement

```bash
go test ./...
go vet ./...
go build ./...
git diff --check
```

## Licence et responsabilité

Consultez les fichiers de licence du dépôt. Utilisez ce logiciel conformément aux conditions de Cloudflare, aux lois applicables et aux règles des services ciblés.

## Références

- [Cloudflare Workers](https://developers.cloudflare.com/workers/)
- [Go TLS package](https://pkg.go.dev/crypto/tls)
- [HTTP CONNECT](https://developer.mozilla.org/en-US/docs/Web/HTTP/Methods/CONNECT)

---

[Lire cette documentation en anglais](README.en.md)
