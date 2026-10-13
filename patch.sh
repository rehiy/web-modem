#!/bin/sh
#

modprobe option

# USB IDs can be overridden, for example:
# MODEM_USB_IDS="2ecc:3012,1234:5678" ./patch.sh
if [ "${MODEM_USB_IDS+x}" = "x" ]; then
    usb_ids="$MODEM_USB_IDS"
else
    usb_ids="2ecc:3012"
fi

for usb_id in $(printf '%s' "$usb_ids" | tr ',;' '  '); do
    case "$usb_id" in
        *:*) echo "${usb_id%:*} ${usb_id#*:}" > /sys/bus/usb-serial/drivers/option1/new_id 2>/dev/null || true ;;
    esac
done

# disable the network interface to avoid conflicts
if [ "${MODEM_DISABLE_IFACE_MACS+x}" = "x" ]; then
    disabled_macs="$MODEM_DISABLE_IFACE_MACS"
else
    disabled_macs="ac:0c:29:a3:9b:6d"
fi

for mac in $(printf '%s' "$disabled_macs" | tr ',;' '  '); do
    for addr in /sys/class/net/*/address; do
        [ -e "$addr" ] || continue
        if [ "$(tr '[:upper:]' '[:lower:]' < "$addr")" = "$(printf '%s' "$mac" | tr '[:upper:]' '[:lower:]')" ]; then
            iface="$(basename "$(dirname "$addr")")"
            [ "$iface" = "lo" ] && continue
            ip link set "$iface" down || true
        fi
    done
done
