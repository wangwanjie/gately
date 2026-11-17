docker exec sut sh -c "cat /app/package.json | grep version; nginx -v 2>&1"
