go build -o fileserver server.go
echo "Install fileserver to /usr/local/bin"
cp -i ./fileserver /usr/local/bin
