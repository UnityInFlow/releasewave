# typed: false
# frozen_string_literal: true

# Homebrew formula for ReleaseWave
# To install: brew install UnityInFlow/tap/releasewave
class Releasewave < Formula
  desc "Universal release/version aggregator for microservices with MCP server support"
  homepage "https://github.com/UnityInFlow/releasewave"
  version "0.1.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/UnityInFlow/releasewave/releases/download/v#{version}/releasewave_#{version}_darwin_arm64.tar.gz"
      sha256 "PLACEHOLDER"
    end

    on_intel do
      url "https://github.com/UnityInFlow/releasewave/releases/download/v#{version}/releasewave_#{version}_darwin_amd64.tar.gz"
      sha256 "PLACEHOLDER"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/UnityInFlow/releasewave/releases/download/v#{version}/releasewave_#{version}_linux_arm64.tar.gz"
      sha256 "PLACEHOLDER"
    end

    on_intel do
      url "https://github.com/UnityInFlow/releasewave/releases/download/v#{version}/releasewave_#{version}_linux_amd64.tar.gz"
      sha256 "PLACEHOLDER"
    end
  end

  def install
    bin.install "releasewave"
  end

  test do
    assert_match "releasewave", shell_output("#{bin}/releasewave version")
  end
end
