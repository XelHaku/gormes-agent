class Gormes < Formula
  desc "Self-improving Go-native AI agent that creates skills from experience"
  homepage "https://gormes.ai"
  # Stable source should point at the semver-named source tarball attached by
  # the Gormes release pipeline, not the CalVer tag tarball. The Go port
  # builds from source so there is no sdist — a plain git archive works.
  url "https://github.com/TrebuchetDynamics/gormes-agent/releases/download/v0.2.0-scout/gormes-0.2.0.tar.gz"
  sha256 "<replace-with-release-asset-sha256>"
  license "MIT"
  head "https://github.com/TrebuchetDynamics/gormes-agent.git", branch: "main"

  depends_on "go" => :build

  def install
    # The Go module lives under `gormes/` inside the repo (the upstream
    # Python code remains at the repo root during the port). Build from
    # there so `./cmd/gormes` resolves correctly.
    cd "gormes" do
      # Mirror the Makefile's static-binary contract so `brew install gormes`
      # produces the same artifact as `make build`:
      #   CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/gormes ./cmd/gormes
      ENV["CGO_ENABLED"] = "0"
      system "go", "build",
        "-trimpath",
        "-ldflags", "-s -w",
        "-o", libexec/"bin/gormes",
        "./cmd/gormes"

      # Ship the persona template alongside the binary so first-run installs
      # outside of the OCI image still have a SOUL.md to seed from. Matches
      # the `docker/SOUL.md` bootstrap path so brew + docker stay aligned.
      pkgshare.install "docker/SOUL.md"
    end

    # Wrap the binary so managed installs know to defer upgrades to brew.
    # Mirrors the upstream `HERMES_MANAGED=homebrew` contract — downstream
    # docs and any future `gormes update` subcommand can branch on this env
    # var to tell the operator "run `brew upgrade gormes` instead".
    (bin/"gormes").write_env_script(
      libexec/"bin/gormes",
      GORMES_MANAGED:      "homebrew",
      GORMES_BUNDLED_SOUL: pkgshare/"SOUL.md"
    )
  end

  test do
    # The cobra-wired `gormes version` command in cmd/gormes/version.go
    # prints `gormes <Version>`. The smoke test pins that contract so a
    # future refactor that accidentally drops the binary name surfaces here.
    assert_match "gormes", shell_output("#{bin}/gormes version")
  end
end
