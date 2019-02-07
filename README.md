# imgdeflator

## Testing

```Shell
> echo -n "s3://nitro-junk/imgdeflator.jpg" | base64 | tr '=' '\0' | xargs -I {} echo "http://127.0.0.1:8080/{}"
http://127.0.0.1:8080/czM6Ly9uaXRyby1qdW5rL2ltZ2RlZmxhdG9yLmpwZw
```

```Shell
> curl -v -H "Content-Type: image/jpeg" --data-binary "@resources/tweety.jpg" "http://127.0.0.1:8080/czM6Ly9uaXRyby1qdW5rL2ltZ2RlZmxhdG9yLmpwZw?width=1024"
```