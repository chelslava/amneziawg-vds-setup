BeforeAll {
  $scriptPath = Join-Path $PSScriptRoot '..\Install-AmneziaWG.ps1'
  $scriptText = Get-Content -LiteralPath $scriptPath -Raw
}
Describe 'Install-AmneziaWG contract' {
  It 'parses' { $t=$null; $e=$null; [void][System.Management.Automation.Language.Parser]::ParseFile($scriptPath,[ref]$t,[ref]$e); $e | Should -BeNullOrEmpty }
  It 'keeps host networking and both required mounts' { $scriptText | Should -Match '--network host'; $scriptText | Should -Match '/etc/amnezia/amneziawg'; $scriptText | Should -Match '/etc/wireguard' }
  It 'contains safe lifecycle and verification controls' { $scriptText | Should -Match 'Reconfigure сбрасывает пароль'; $scriptText | Should -Match 'ss -lunH'; $scriptText | Should -Match 'RESULT_PASSWORD' }
  It 'generates and persists a per-install AmneziaWG signature' { $scriptText | Should -Match 'generate_awg_params'; $scriptText | Should -Match 'rand_range 3 6'; $scriptText | Should -Match 'JC=%s'; $scriptText | Should -Match 'JMIN=%s'; $scriptText | Should -Match 'JMAX=%s'; $scriptText | Should -Match 'S1=%s'; $scriptText | Should -Match 'S2=%s'; $scriptText | Should -Match 'H4=%s' }
}
