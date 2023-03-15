#!/bin/bash

sudo cp ../sneaky-tunnel.service /etc/systemd/system/sneaky-tunnel.service
sudo chmod 664 /etc/systemd/system/sneaky-tunnel.service
sudo systemctl daemon-reload
sudo systemctl start sneaky-tunnel
sudo systemctl enable sneaky-tunnel
sudo systemctl status sneaky-tunnel