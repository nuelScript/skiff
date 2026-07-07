package docker

func (e *Engine) Stop(container string) error {
	out, err := e.command("stop", container).CombinedOutput()
	if err != nil {
		return cmdErr(out, err)
	}
	return nil
}

func (e *Engine) Remove(name string) error {
	out, err := e.command("rm", "-f", name).CombinedOutput()
	if err != nil {
		return cmdErr(out, err)
	}
	return nil
}

// Tag adds an additional name to an existing image, so a build can be retained
// as a rollback point (e.g. skiff-app:latest -> skiff-app:<deployid>).
func (e *Engine) Tag(src, dst string) error {
	out, err := e.command("tag", src, dst).CombinedOutput()
	if err != nil {
		return cmdErr(out, err)
	}
	return nil
}

// AppImageTags lists the retained tags of an app's images (skiff-<app>:*),
// newest first, excluding :latest and dangling <none>. Docker lists images
// created-descending by default, which is the order we rely on for pruning.
func (e *Engine) AppImageTags(app string) []string {
	out, err := e.command("images", "skiff-"+app, "--format", "{{.Tag}}").Output()
	if err != nil {
		return nil
	}
	var tags []string
	for _, t := range splitLines(out) {
		if t == "latest" || t == "<none>" {
			continue
		}
		tags = append(tags, t)
	}
	return tags
}

// ImageExists reports whether a tagged image is present locally.
func (e *Engine) ImageExists(tag string) bool {
	return e.command("image", "inspect", tag).Run() == nil
}

// RemoveImage deletes a tagged image (best-effort; ignores "in use").
func (e *Engine) RemoveImage(tag string) error {
	return e.command("rmi", tag).Run()
}
