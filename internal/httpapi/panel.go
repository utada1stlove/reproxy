package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var panelAssets embed.FS

func panelAssetHandler() http.Handler {
	subtree, err := fs.Sub(panelAssets, "static")
	if err != nil {
		panic(err)
	}

	return http.StripPrefix("/panel/", http.FileServerFS(subtree))
}
