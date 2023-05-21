# scurl
Like `curl` but a server. This tool lets you quickly serve a simple http-response 
in the same way that `curl` lets you do a http-request.

It allows you to serve the same response n-number of times, and the server will 
exit when the number of responses has been sent.

It will not filer on `host` or `path` any http-request will be served the same 
response.


# Basic usage
```bash
surl -d '{"error_code":"-1","message":"dunno"}' \
    -H 'Content-Type: application/json' -s 500 \
    :8080
```
