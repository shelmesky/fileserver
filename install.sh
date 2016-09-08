go build -o fileserver server.go
echo "Install fileserver to /usr/local/bin"
sudo cp -i ./fileserver /usr/local/bin
