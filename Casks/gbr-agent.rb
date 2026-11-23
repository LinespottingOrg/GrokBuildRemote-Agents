cask "gbr-agent" do
  arch arm: "arm64", intel: "x64"

  version "0.6.8"
  sha256 arm:   "f22963f20567567cf4a3ad8123cd34f60e9b534ff9c5a810196da07801ff95de",
         intel: "d32173b7ec893aa18e83dceac3334c4f637942917db90000f4c986f37da30fad"

  url "https://grokbuildremote.com/downloads/packages/#{version}/GBR_#{version}-darwin-#{arch}.tar.xz"
  name "Build Remote Agent"
  desc "Desktop agent for Build Remote Agent"
  homepage "https://grokbuildremote.com/"

  binary "gbr-agent"
end
