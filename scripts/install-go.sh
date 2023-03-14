#!/bin/bash

cd
curl -OL https://golang.org/dl/go1.19.3.linux-amd64.tar.gz
sudo tar -C /usr/local -xvf go1.19.3.linux-amd64.tar.gz
rm go1.19.3.linux-amd64.tar.gz
echo "export PATH=$PATH:/usr/local/go/bin" >> ~/.profile
source ~/.profile
go version