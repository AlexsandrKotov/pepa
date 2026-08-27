# Homebrew formula for PEPA CLI
# Install: brew install pepa/tap/pepa-cli
#
# This formula is auto-updated by the release workflow.
# For development, use: brew install --build-from-source pepa-cli

class PepaCli < Formula
  desc "PEPA — Platform Engineering & Pipeline Automator CLI"
  homepage "https://github.com/akotau/pepa"
  url "https://github.com/akotau/pepa/releases/download/v0.1.0/pepa-v0.1.0-darwin-arm64.tar.gz"
  sha256 "REPLACE_WITH_ACTUAL_SHA256"
  license "Apache-2.0"

  livecheck do
    url :stable
    strategy :github_latest
  end

  bottle :unneeded

  def install
    bin.install "pepa"
  end

  test do
    output = shell_output("#{bin}/pepa version 2>&1")
    assert_match "pepa version", output
  end
end
