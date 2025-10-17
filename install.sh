#!/bin/bash

# Install script for Video Compressor - builds package and enables service

set -e

echo "Building and installing compressor package..."
makepkg -si

echo "Enabling and starting the user service..."
systemctl --user enable compressor.service
systemctl --user start compressor.service

echo "Service status:"
systemctl --user status compressor.service

echo ""
echo "Installation complete!"
echo "To check logs: journalctl --user -u compressor.service -f"
echo "To stop: systemctl --user stop compressor.service"
echo "To restart: systemctl --user restart compressor.service"
echo ""
echo "Configuration: Copy /etc/compressor.env to ~/compressor.env and edit as needed"