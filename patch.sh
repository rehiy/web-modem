#!/bin/sh
#

modprobe option
echo 2ecc 3012 > /sys/bus/usb-serial/drivers/option1/new_id

# disable the network interface to avoid conflicts
ip -o link show | grep -i "ac:0c:29:a3:9b:6d" | awk -F': ' '{print $2}' | xargs -I {} ip link set {} down
