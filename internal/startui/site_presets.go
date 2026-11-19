package startui

// These are editable starting points, never hidden additions to a submitted
// policy. Container layers can redirect to vendor CDNs. Keep hosts scoped to
// the services used here instead of allowing arbitrary cloud-storage domains.
// References: Docker Desktop allow-list, GitHub Actions runner communication,
// Microsoft Container Registry firewall guidance, and VS Code network docs.
func sitePresets() map[string][]string {
	common := []string{
		"docker.io", "registry-1.docker.io", "auth.docker.io", "hub.docker.com",
		"production.cloudflare.docker.com", "production.cloudfront.docker.com",
		"docker-images-prod.6aa30f8b08e16409b46e0173d6de2f56.r2.cloudflarestorage.com",
		"ghcr.io", "pkg-containers.githubusercontent.com", "github-registry-files.githubusercontent.com",
		"mcr.microsoft.com", "*.data.mcr.microsoft.com",
		"quay.io", "*.quay.io",
		"registry.npmjs.org", "nodejs.org", "containers.dev",
		"github.com", "api.github.com", "raw.githubusercontent.com", "codeload.github.com",
		"objects.githubusercontent.com", "release-assets.githubusercontent.com", "github-releases.githubusercontent.com",
		"deb.debian.org", "security.debian.org", "archive.ubuntu.com", "*.archive.ubuntu.com",
		"security.ubuntu.com", "ports.ubuntu.com",
		"update.code.visualstudio.com", "vscode.download.prss.microsoft.com", "vscode.blob.core.windows.net",
		"marketplace.visualstudio.com", "*.gallery.vsassets.io", "*.gallerycdn.vsassets.io", "*.vscode-cdn.net", "*.vscode-unpkg.net",
	}
	python := []string{"pypi.org", "pypi.python.org", "files.pythonhosted.org", "repo.anaconda.com", "conda.anaconda.org"}
	r := []string{"cloud.r-project.org", "cran.r-project.org", "cran.rstudio.com", "packagemanager.posit.co", "rspm-sync.rstudio.com",
		"bioconductor.org", "*.bioconductor.org", "bioconductor.posit.co", "bioconductor.statistik.tu-dortmund.de", "ftp.gwdg.de", "mghp.osn.xsede.org", "r-universe.dev", "*.r-universe.dev"}
	return map[string][]string{
		"python-sandbox":         mergeSites(common, python),
		"r-sandbox":              mergeSites(common, r),
		"bioinformatics-sandbox": mergeSites(common, python, r),
	}
}

func mergeSites(groups ...[]string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, group := range groups {
		for _, host := range group {
			if !seen[host] {
				result = append(result, host)
				seen[host] = true
			}
		}
	}
	return result
}
