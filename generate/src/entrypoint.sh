#!/bin/sh

set -e

LOOPBACK_DEVICE=$(losetup -f)

function size() {
  du -sm ./ | cut -f1
}

function iso() {
  mkdir -p ./boot/isolinux
  cp /usr/local/src/syslinux/bios/core/isolinux.bin ./boot/isolinux/isolinux.bin
  cp /usr/local/src/syslinux/bios/com32/elflink/ldlinux/ldlinux.c32 ./boot/isolinux/ldlinux.c32
  cat <<EOF >./boot/isolinux/isolinux.cfg
DEFAULT Dianemo
  SAY Dianemo
LABEL Dianemo
  KERNEL /boot/vmlinuz
  INITRD /boot/initramfs.xz
  APPEND ip=dhcp consoleblank=0 console=tty0 console=ttyS0,9600 dianemo.autonomy.io/root=/dev/xvda
EOF
  mkisofs -o /out/dianemo.iso -b boot/isolinux/isolinux.bin -c boot/isolinux/boot.cat -no-emul-boot -boot-load-size 4 -boot-info-table .
}

function raw() {
  dd if=/dev/zero of=/dianemo.raw bs=1M count=$(($(size) + 150))
  parted -s /dianemo.raw mklabel gpt
  parted -s -a optimal /dianemo.raw mkpart ESP fat32 0 50M
  parted -s -a optimal /dianemo.raw mkpart ROOT xfs 50M $(($(size) + 100))M
  parted -s -a optimal /dianemo.raw mkpart DATA xfs $(($(size) + 100))M 100%
  losetup ${LOOPBACK_DEVICE} /dianemo.raw
  partx -av ${LOOPBACK_DEVICE}
  sgdisk ${LOOPBACK_DEVICE} --attributes=1:set:2
  mkfs.vfat ${LOOPBACK_DEVICE}p1
  mkfs.xfs -n ftype=1 -L ROOT ${LOOPBACK_DEVICE}p2
  mkfs.xfs -n ftype=1 -L DATA ${LOOPBACK_DEVICE}p3
  mount ${LOOPBACK_DEVICE}p1 /mnt
  mkdir -p /mnt/boot/extlinux
  extlinux --install /mnt/boot/extlinux
  cat <<EOF >/mnt/boot/extlinux/extlinux.conf
DEFAULT Dianemo
  SAY Dianemo by Autonomy
LABEL Dianemo
  KERNEL /boot/vmlinuz
  INITRD /boot/initramfs.xz
  APPEND ip=dhcp consoleblank=0 console=tty0 console=ttyS0,9600 dianemo.autonomy.io/root=/dev/xvda
EOF
  cp -v /rootfs/boot/* /mnt/boot
  umount /mnt
  mount ${LOOPBACK_DEVICE}p2 /mnt
  cp -Rv ./* /mnt
  rm -rf /mnt/boot
  rm -rf /mnt/var/*
  umount /mnt
  mount ${LOOPBACK_DEVICE}p3 /mnt
  cp -Rv ./var/* /mnt
  dd if=/usr/local/src/syslinux/efi64/mbr/gptmbr.bin of=${LOOPBACK_DEVICE}
  cleanup
  cp -v /dianemo.raw /out
  qemu-img convert -f raw -O vmdk /out/dianemo.raw /out/dianemo.vmdk
}

function rootfs() {
  dd if=/dev/zero of=/rootfs.raw bs=1M count=$(($(size) + 100))
  parted -s /rootfs.raw mklabel gpt
  parted -s -a optimal /rootfs.raw mkpart ROOT xfs 0 $(($(size) + 50))M
  parted -s -a optimal /rootfs.raw mkpart DATA xfs $(($(size) + 50))M 100%
  losetup ${LOOPBACK_DEVICE} /rootfs.raw
  partx -av ${LOOPBACK_DEVICE}
  mkfs.xfs -n ftype=1 -L ROOT ${LOOPBACK_DEVICE}p1
  mkfs.xfs -n ftype=1 -L DATA ${LOOPBACK_DEVICE}p2
  mount ${LOOPBACK_DEVICE}p1 /mnt
  cp -Rv ./* /mnt
  rm -rf /mnt/boot
  rm -rf /mnt/var/*
  umount /mnt
  mount ${LOOPBACK_DEVICE}p2 /mnt
  cp -Rv ./var/* /mnt
  cleanup
  cp -v ./boot/vmlinuz /out
  cp -v ./boot/initramfs.xz /out
  cp -v /rootfs.raw /out
}

function cleanup {
  umount /mnt || true
  partx -dv ${LOOPBACK_DEVICE} || true
  losetup -d ${LOOPBACK_DEVICE} || true
}

trap cleanup EXIT

case "$1" in
        raw)
            rootfs
            raw
            iso
            ;;
        iso)
            iso
            ;;
        *)
            exec "$@"
esac
