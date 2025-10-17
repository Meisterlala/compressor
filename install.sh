#!/bin/bash

# Install script for Video Compressor - builds package and enables service

set -e

echo "Building and installing compressor package..."
makepkg -si --noextract

echo "Enabling and starting the user service..."
systemctl --user enable compressor.service
systemctl --user start compressor.service

echo "Service status:"
systemctl --user status compressor.service
