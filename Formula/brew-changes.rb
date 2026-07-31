class BrewChanges < Formula
  desc "Interactive changelog viewer for outdated Homebrew packages"
  homepage "https://github.com/garthdw/homebrew-changes"
  license "MIT"
  head "https://github.com/garthdw/homebrew-changes.git", branch: "main"

  # No tagged release yet — install with `brew install --HEAD garthdw/changes/brew-changes`.
  # Once a version is tagged, add a `url`/`sha256` stable block here pointing at that tag.

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args, "./cmd/brew-changes"
  end

  test do
    assert_match "Checking for outdated packages", shell_output("#{bin}/brew-changes 2>&1", 0..1)
  end
end
