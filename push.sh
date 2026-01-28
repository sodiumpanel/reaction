#!/bin/bash
echo "Enter your GitHub Personal Access Token:"
read -s TOKEN
git remote set-url origin https://$TOKEN@github.com/zt3xdv/sodium-wings.git
git push -u origin main
git remote set-url origin https://github.com/zt3xdv/sodium-wings.git
echo "Done!"
