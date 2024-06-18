#!/usr/bin/env bash
set -eu
image=$1
creds=${2-}

# https://github.com/moby/moby/blob/v20.10.18/vendor/github.com/docker/distribution/reference/normalize.go#L29-L57
# https://github.com/moby/moby/blob/v20.10.18/vendor/github.com/docker/distribution/reference/normalize.go#L88-L105
registry=${image%%/*}
if [ "$registry" = "$image" ] \
|| { [ "`expr index "$registry" .:`" = 0 ] && [ "$registry" != localhost ]; }; then
    registry=docker.io
else
    image=${image#*/}
fi
if [ "$registry" = docker.io ] && [ "`expr index "$image" /`" = 0 ]; then
    image=library/$image
fi
if [ "`expr index "$image" :`" = 0 ]; then
    tag=latest
else
    tag=${image#*:}
    image=${image%:*}
fi
if [ "$registry" = docker.io ]; then
    registry=https://registry-1.docker.io
elif ! [[ "$registry" =~ ^localhost(:[0-9]+)$ ]]; then
    registry=https://$registry
fi

r=`curl -sS "$registry/v2/" \
    -o /dev/null \
    -w '%{http_code}:%header{www-authenticate}'`
http_code=`echo "$r" | cut -d: -f1`
curl_args=(-sS -H 'Accept: application/vnd.docker.distribution.manifest.v2+json')
if [ "$http_code" = 401 ]; then
    if [ "$registry" = https://registry-1.docker.io ]; then
        header_www_authenticate=`echo "$r" | cut -d: -f2-`
        header_www_authenticate=`echo "$header_www_authenticate" | sed -E 's/^Bearer +//'`
        split_into_lines() {
            sed -Ee :1 -e 's/^(([^",]|"([^"]|\")*")*),/\1\n/; t1'
        }
        header_www_authenticate=`echo "$header_www_authenticate" | split_into_lines`
        extract_value() {
            sed -E 's/^[^=]+="(([^"]|\")*)"$/\1/; s/\\(.)/\1/g'
        }
        realm=$(echo "$header_www_authenticate" | grep '^realm=' | extract_value)
        service=$(echo "$header_www_authenticate" | grep '^service=' | extract_value)
        scope=repository:$image:pull
        token=`curl -sS "$realm?service=$service&scope=$scope" | jq -r .token`
        curl_args+=(-H "Authorization: Bearer $token")
    else
        curl_args+=(-u "$creds")
    fi
fi
manifest=`curl "${curl_args[@]}" "$registry/v2/$image/manifests/$tag"`
config_digest=`echo "$manifest" | jq -r .config.digest`
config=`curl "${curl_args[@]}" -L "$registry/v2/$image/blobs/$config_digest"`
layers=`echo "$manifest" | jq -r '.layers[] | .digest'`
echo "$layers" | \
    while IFS= read -r digest; do
        curl "${curl_args[@]}" -L "$registry/v2/$image/blobs/$digest" | wc -c
    done
