# Sonda del SDK de AWS (soporte de las decisiones 10, 11 y 12 de `gcsgrep-s3-spec.md`)

Programa descartable, **no** forma parte del módulo `gcsgrep`. Levanta un servidor HTTP falso
que responde `404 NoSuchBucket`, apunta el SDK a él con `AWS_ENDPOINT_URL_S3` y llama
`config.LoadDefaultConfig`, `cfg.Credentials.Retrieve` y `ListObjectsV2`.
Versiones: `aws-sdk-go-v2 v1.47.1`, `config v1.33.7`, `service/s3 v1.114.1`, Go 1.26.1.

## Código

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func main() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("REQ", r.Method, r.URL.String(), "host", r.Host, "AE", r.Header.Get("Accept-Encoding"))
		w.WriteHeader(404)
		w.Write([]byte(`<?xml version="1.0"?><Error><Code>NoSuchBucket</Code><Message>x</Message></Error>`))
	}))
	defer srv.Close()
	fmt.Println("endpoint", srv.URL)
	ctx := context.Background()
	if os.Getenv("USE_EP") == "1" { os.Setenv("AWS_ENDPOINT_URL_S3", srv.URL) }
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRetryMaxAttempts(1))
	fmt.Println("cfgerr", err, "region", cfg.Region)
	_, err = cfg.Credentials.Retrieve(ctx)
	fmt.Println("creds err:", err)
	c := s3.NewFromConfig(cfg)
	_, err = c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String("mybucket"), Prefix: aws.String("p/")})
	fmt.Printf("list err: %T %v\n", err, err)
}
```

## Salida medida (2026-10-08)

Entorno de cada corrida: `env -i HOME=<dir vacío> PATH=/usr/bin:/bin AWS_EC2_METADATA_DISABLED=true USE_EP=1 …`

| Caso | Variables extra | Resultado |
|---|---|---|
| A: sin credenciales, sin región | — | `cfgerr <nil> region ""`; `Retrieve` falla (`failed to refresh cached credentials, no EC2 IMDS role found …`); `ListObjectsV2` falla con `Invalid region: region was not a valid DNS name.` |
| B: credenciales y región | `AWS_ACCESS_KEY_ID=k AWS_SECRET_ACCESS_KEY=s AWS_REGION=us-east-1` | `Retrieve` ok; el servidor recibe `GET /mybucket?list-type=2&prefix=p%2F` con `Accept-Encoding: identity`; el error es `*smithy.OperationError` y su texto incluye `StatusCode: 404 … NoSuchBucket: x` (la clasificación con `errors.As` sobre `types.NoSuchBucket` la prueba la iteración 3, no la sonda) |
| C: credenciales, sin región | `AWS_ACCESS_KEY_ID=k AWS_SECRET_ACCESS_KEY=s` | `Retrieve` ok; `ListObjectsV2` falla con `Invalid region: region was not a valid DNS name.` |

Nota sobre el caso A: `no EC2 IMDS role found … access disabled` es el último eslabón de la cadena de credenciales (el proveedor de metadata de EC2, deshabilitado por `AWS_EC2_METADATA_DISABLED=true`); significa que ninguna fuente anterior (variables, archivos) dio credenciales.
