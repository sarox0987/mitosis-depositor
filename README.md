# Install git

```
sudo apt-get install git
```

# Install go

```
VERSION="1.22.9"
ARCH="amd64"
curl -O -L "https://golang.org/dl/go${VERSION}.linux-${ARCH}.tar.gz"
tar -xf "go${VERSION}.linux-${ARCH}.tar.gz"
sudo chown -R root:root ./go
sudo mv -v go /usr/local
export GOPATH=$HOME/go
export PATH=$PATH:/usr/local/go/bin:$GOPATH/bin
source ~/.bash_profile
go version
```

# Clone Repository

```
https://github.com/sarox0987/mitosis-depositor.git
cd mitosis-depositor
```

# Run the script

```
go mod tidy
go run main.go
```