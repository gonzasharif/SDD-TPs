# Sonda del SDK de AWS (soporte de las decisiones de `gcsgrep-s3-spec.md`)

Programa descartable, **no** forma parte del módulo `gcsgrep`. Levanta un servidor HTTP falso
que responde según `MODE`, apunta el SDK a él con `AWS_ENDPOINT_URL_S3` y llama
`config.LoadDefaultConfig`, `cfg.Credentials.Retrieve`, `ListObjectsV2` o `GetObject`.
Versiones: `aws-sdk-go-v2 v1.47.1`, `config v1.33.7`, `credentials v1.20.7`,
`service/s3 v1.114.1`, `smithy-go v1.28.1`, Go 1.26.1 (linux/amd64). Medido el 2026-10-08.

Entorno de cada corrida: `env -i HOME=<dir vacío> PATH=/usr/bin:/bin AWS_EC2_METADATA_DISABLED=true USE_EP=1 …`
(más `AWS_ACCESS_KEY_ID=k AWS_SECRET_ACCESS_KEY=s AWS_REGION=us-east-1` salvo donde se indica).
`config.WithRetryMaxAttempts(1)` en todas las corridas.

## Salida medida

| Caso (`MODE`) | Resultado |
|---|---|
| Sin credenciales ni región (`list404`) | `LoadDefaultConfig` devuelve `nil` y región `""`; `Credentials.Retrieve` falla con `failed to refresh cached credentials, no EC2 IMDS role found …`; `ListObjectsV2` falla con `Invalid region: region was not a valid DNS name.` |
| Credenciales sin región | `Retrieve` ok; `ListObjectsV2` falla con `Invalid region: region was not a valid DNS name.` |
| `list404` (404 `NoSuchBucket`) | request `GET /mybucket?list-type=2&prefix=p%2F`, `Accept-Encoding: identity`; `smithy.APIError` con código `NoSuchBucket` |
| `list403` (403 `AccessDenied`) | `smithy.APIError` con código `AccessDenied`, `StatusCode: 403` |
| `list503` (503 `SlowDown`) | `smithy.APIError` con código `SlowDown`; el texto incluye `exceeded maximum number of attempts, 1 … StatusCode: 503` |
| `get404` (404 `NoSuchKey`) | request `GET /mybucket/p/a.log?x-id=GetObject`; código `NoSuchKey` |
| `get403` (403 `AccessDenied`) | código `AccessDenied`, `StatusCode: 403` |
| `getarch` (403 `InvalidObjectState`) | código `InvalidObjectState`, `StatusCode: 403`, texto `The operation is not valid for the object's storage class` |
| `getok` con `Content-Encoding: gzip` | el cuerpo llega sin decodificar (`"timeout\n"`); el SDK escribe en **stderr** la línea `SDK <fecha> DEBUG Response has no supported checksum. Not validating response payload.` (1 línea por GET) |
| `getcut` (`Content-Length: 1000`, llegan 17 bytes y se cierra) | `ReadAll` devuelve los 17 bytes y el error `unexpected EOF` |
| `getrst` (cierre con RST) | `request send failed, Get "…": read tcp …: read: connection reset by peer` |
| `getbig` 5 MiB / 500 MiB (`io.Copy(io.Discard, Body)`) | RSS máximo del proceso (cliente y servidor juntos): 12.356 KiB / 12.612 KiB (diferencia: 256 KiB) |
| Sin credenciales y **sin** `AWS_EC2_METADATA_DISABLED` (`env -i HOME=<dir vacío> PATH=/usr/bin:/bin USE_EP=1 AWS_REGION=us-east-1 MODE=list404`, 2 corridas, `/usr/bin/time`) | `Retrieve` falla con `… ec2imds: GetMetadata, request canceled, context deadline exceeded` tras 7,77 s y 7,28 s de reloj |
| endpoint IP (`127.0.0.1:<puerto>`) | el SDK usa estilo de ruta (`/<bucket>/<clave>`), con o sin `UsePathStyle` |

## Código

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

func xml(code, msg string) string {
	return `<?xml version="1.0"?><Error><Code>` + code + `</Code><Message>` + msg + `</Message></Error>`
}

func main() {
	mode := os.Getenv("MODE") // list404 list403 list503 get404 get403 getok
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("REQ", r.Method, r.URL.String(), "AE=", r.Header.Get("Accept-Encoding"))
		switch mode {
		case "list404":
			w.WriteHeader(404); io.WriteString(w, xml("NoSuchBucket", "x"))
		case "list403":
			w.WriteHeader(403); io.WriteString(w, xml("AccessDenied", "x"))
		case "list503":
			w.WriteHeader(503); io.WriteString(w, xml("SlowDown", "x"))
		case "get404":
			w.WriteHeader(404); io.WriteString(w, xml("NoSuchKey", "x"))
		case "get403":
			w.WriteHeader(403); io.WriteString(w, xml("AccessDenied", "x"))
		case "getarch":
			w.WriteHeader(403); io.WriteString(w, xml("InvalidObjectState", "The operation is not valid for the object's storage class"))
		case "getcut":
			w.Header().Set("Content-Length", "1000"); w.WriteHeader(200); io.WriteString(w, "timeout uno\nINFO\n")
			if f, ok := w.(http.Flusher); ok { f.Flush() }
			if hj, ok := w.(http.Hijacker); ok { c, _, _ := hj.Hijack(); c.Close() }
		case "getbig":
			n := 500
			if os.Getenv("MIB") != "" { fmt.Sscan(os.Getenv("MIB"), &n) }
			w.Header().Set("Content-Length", fmt.Sprint(n<<20))
			line := make([]byte, 1024); for i := range line { line[i] = 'x' }; line[1023] = '\n'
			for i := 0; i < n<<10; i++ { w.Write(line) }
		case "getrst":
			if hj, ok := w.(http.Hijacker); ok { c, _, _ := hj.Hijack(); c.(*net.TCPConn).SetLinger(0); c.Close() }
		case "getok":
			w.Header().Set("Content-Length", "8"); w.Header().Set("Content-Encoding", "gzip")
			io.WriteString(w, "timeout\n")
		case "listok":
			io.WriteString(w, `<?xml version="1.0"?><ListBucketResult><Name>b</Name><Prefix>p/</Prefix><KeyCount>1</KeyCount><IsTruncated>false</IsTruncated><Contents><Key>p/a.log</Key><Size>8</Size></Contents></ListBucketResult>`)
		}
	}))
	defer srv.Close()
	if os.Getenv("USE_EP") == "1" { os.Setenv("AWS_ENDPOINT_URL_S3", srv.URL) }
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRetryMaxAttempts(1))
	fmt.Println("cfgerr", err, "region", cfg.Region)
	_, err = cfg.Credentials.Retrieve(ctx)
	fmt.Println("creds err:", err)
	c := s3.NewFromConfig(cfg, func(o *s3.Options){ o.UsePathStyle = os.Getenv("PATH_STYLE")=="1" })
	if mode[:4] == "list" {
		_, err = c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String("mybucket"), Prefix: aws.String("p/")})
	} else {
		var out *s3.GetObjectOutput
		out, err = c.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String("mybucket"), Key: aws.String("p/a.log")})
		if err == nil {
			if mode == "getbig" { n, e := io.Copy(io.Discard, out.Body); fmt.Println("read", n, e); var ru syscall.Rusage; syscall.Getrusage(0, &ru); fmt.Println("maxrss KiB", ru.Maxrss) } else {
			b, e := io.ReadAll(out.Body); fmt.Printf("body %q readerr=%v\n", b, e) }
		}
	}
	var ae smithy.APIError
	if errors.As(err, &ae) { fmt.Println("apierr code:", ae.ErrorCode()) }
	fmt.Printf("err: %T %v\n", err, err)
}
```
