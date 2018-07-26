// +build go1.9

package main

var (
	// Note the -skip needs to be first
	tczList = []string{
		"-skip", "graphics-KERNEL alsa-modules-KERNEL wireless-KERNEL",
		"alsa", "alsa-config", "alsa-plugins",
		"aterm",
		"bash",
		"strace",
		"freetype",
		"glib2",
		"harfbuzz",
		"imlib2-bin",
		"imlib2",
		"libffi",
		"libfontenc",
		"libICE",
		"libjpeg-turbo",
		"libpng",
		"libSM",
		"libX11",
		"libXau",
		"libxcb",
		"libXdmcp",
		"libXext",
		"libXfont",
		"libXi",
		"libXmu",
		"libXpm",
		"libXrandr",
		"libXrender",
		"libXt",
		"pcre",
		"wbar",
		"Xfbdev",
		"xf86-video-intel",
		"Xlibs",
		"Xorg-fonts",
		"Xprogs",
		"Xorg-7.7",
		"wifi",
	}
)
