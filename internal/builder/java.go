package builder

import "path/filepath"

type javaBuilder struct{ dir string }

func (j *javaBuilder) Name() string { return "Java" }

func (j *javaBuilder) detect() bool {
	for _, f := range []string{"pom.xml", "build.gradle", "build.gradle.kts", "Main.java"} {
		if fileExists(filepath.Join(j.dir, f)) {
			return true
		}
	}
	return false
}

func (j *javaBuilder) Dockerfile(port int, env map[string]string) (string, error) {
	switch {
	case fileExists(filepath.Join(j.dir, "pom.xml")):
		return render(Plan{
			Base:        "maven:3-eclipse-temurin-21",
			Build:       []string{"mvn -q -DskipTests package"},
			Env:         env,
			RuntimeBase: "eclipse-temurin:21-jre",
			Copy:        []Artifact{{From: "/app/target/*.jar", To: "/app/app.jar"}},
			Start:       []string{"java", "-jar", "/app/app.jar"},
			Port:        port,
		})
	case fileExists(filepath.Join(j.dir, "build.gradle")) || fileExists(filepath.Join(j.dir, "build.gradle.kts")):
		return render(Plan{
			Base:        "gradle:8-jdk21",
			Build:       []string{"gradle -q build -x test"},
			Env:         env,
			RuntimeBase: "eclipse-temurin:21-jre",
			Copy:        []Artifact{{From: "/app/build/libs/*.jar", To: "/app/app.jar"}},
			Start:       []string{"java", "-jar", "/app/app.jar"},
			Port:        port,
		})
	default:
		return render(Plan{
			Base:  "eclipse-temurin:21-jdk",
			Build: []string{"javac Main.java"},
			Env:   env,
			Start: []string{"java", "Main"},
			Port:  port,
		})
	}
}
