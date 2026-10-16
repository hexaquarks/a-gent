class AGent < Formula
  desc "Terminal dashboard for coding agents and subagents across projects"
  homepage "https://github.com/hexaquarks/a-gent"
  license "MIT"
  head "https://github.com/hexaquarks/a-gent.git", branch: "main"

  depends_on "go" => :build
  depends_on "tmux"
  uses_from_macos "lsof"

  def install
    ENV["CGO_ENABLED"] = "0"
    system "go", "build", *std_go_args(ldflags: "-X main.version=HEAD"), "./cmd/a-gent"
  end

  test do
    assert_equal "a-gent HEAD\n", shell_output("#{bin}/a-gent --version")
  end
end
