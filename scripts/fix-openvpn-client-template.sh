#!/bin/bash

PUBLIC_IP=$(curl https://ipinfo.io/ip)

sudo echo >> /etc/openvpn/client-template.txt "route $PUBLIC_IP 255.255.255.255 net_gateway"
sudo echo >> /etc/openvpn/client-template.txt "fast-io"