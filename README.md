# imgproxy

## Testing

```Shell
> echo -n "s3://nitro-junk/imgproxy.jpg" | base64 | tr '=' '\0' | xargs -I {} echo "http://127.0.0.1:8080/{}"
http://127.0.0.1:8080/czM6Ly9uaXRyby1qdW5rL2ltZ3Byb3h5LmpwZw
```

```Shell
> curl -H "Content-Type: image/jpeg" --data-binary '@resources/tweety.jpg' http://127.0.0.1:8080/czM6Ly9uaXRyby1qdW5rL2ltZ3Byb3h5LmpwZw
```