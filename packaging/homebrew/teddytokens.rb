# Template for Formula/teddytokens.rb in AdamFagerberg/homebrew-tap.
# Copy it into the tap, then replace the version in `url` and the sha256 with
# the output of:
#   curl -sL https://github.com/AdamFagerberg/teddytokens/archive/refs/tags/vX.Y.Z.tar.gz | shasum -a 256
class Teddytokens < Formula
  desc "Watch Claude Code token spend pile up as falling pixel-art characters"
  homepage "https://github.com/AdamFagerberg/teddytokens"
  url "https://github.com/AdamFagerberg/teddytokens/archive/refs/tags/vX.Y.Z.tar.gz"
  sha256 "REPLACE_WITH_SHA256_OF_THE_TARBALL"
  license "MIT"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-X main.version=#{version}")
  end

  test do
    assert_match "teddytokens #{version}", shell_output("#{bin}/teddytokens --version")
  end
end
