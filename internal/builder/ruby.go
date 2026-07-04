package builder

import "path/filepath"

type rubyBuilder struct{ dir string }

func (r *rubyBuilder) Name() string { return "Ruby" }

func (r *rubyBuilder) detect() bool {
	for _, f := range []string{"Gemfile", "config.ru", "app.rb", "main.rb"} {
		if fileExists(filepath.Join(r.dir, f)) {
			return true
		}
	}
	return false
}

func (r *rubyBuilder) Dockerfile(port int, env map[string]string) (string, error) {
	var install, cache []string
	if fileExists(filepath.Join(r.dir, "Gemfile")) {
		install = []string{"bundle install"}
		cache = []string{"Gemfile"}
		if fileExists(filepath.Join(r.dir, "Gemfile.lock")) {
			cache = append(cache, "Gemfile.lock")
		}
	}
	return render(Plan{
		Base:       "ruby:3-slim",
		CacheFiles: cache,
		Install:    install,
		Env:        env,
		Start:      r.start(),
		Port:       port,
	})
}

func (r *rubyBuilder) start() []string {
	if fileExists(filepath.Join(r.dir, "config.ru")) {
		return []string{"sh", "-c", "bundle exec rackup -o 0.0.0.0 -p $PORT"}
	}
	for _, f := range []string{"app.rb", "main.rb", "server.rb"} {
		if fileExists(filepath.Join(r.dir, f)) {
			return []string{"ruby", f}
		}
	}
	return []string{"sh", "-c", "ruby app.rb"}
}
