[CmdletBinding()]
param(
  [string] $UpdateManifestUrl = 'https://persistent.oaistatic.com/codex-app-prod/windows-store-update.json',
  [string] $ProductId = '9PLM9XGG6VKS',
  [string] $PackageIdentity = 'OpenAI.Codex',
  [string] $Architecture = 'x64',
  [string] $StatePath = '',
  [string] $LocalMsixPath = '',
  [switch] $DryRun,
  [switch] $ProbeOnly,
  [switch] $Force,
  [string] $ReleaseTagPrefix = 'codex-msix',
  [string] $GitHubRepository = '',
  [string] $GitHubToken = ''
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$script:Fe3HttpClient = $null

if ([string]::IsNullOrWhiteSpace($StatePath)) {
  $scriptDir = $PSScriptRoot
  if ([string]::IsNullOrWhiteSpace($scriptDir) -and $MyInvocation.MyCommand.Path) {
    $scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
  }

  if ([string]::IsNullOrWhiteSpace($scriptDir)) {
    $scriptDir = (Get-Location).Path
  }

  $StatePath = Join-Path $scriptDir '..\data\latest.json'
}

try {
  Add-Type -AssemblyName System.IO.Compression.FileSystem | Out-Null
} catch {
  try {
    Add-Type -AssemblyName System.IO.Compression | Out-Null
  } catch {
    # The compression types may already be available in some hosts.
  }
}

try {
  Add-Type -AssemblyName System.Net.Http | Out-Null
} catch {
  # The HTTP types may already be available in some hosts.
}

function Write-Log {
  param(
    [string] $Level,
    [string] $Message
  )

  switch ($Level) {
    'WARN' { Write-Host "[warn] $Message" -ForegroundColor Yellow }
    'ERROR' { Write-Host "[error] $Message" -ForegroundColor Red }
    default { Write-Host "[info] $Message" -ForegroundColor Cyan }
  }
}

function Test-CommandExists {
  param([string] $Name)
  return [bool](Get-Command $Name -ErrorAction SilentlyContinue)
}

function Get-UtcTimestamp {
  return [DateTime]::UtcNow.ToString("yyyy-MM-ddTHH:mm:ssZ")
}

function Get-ObjectPropertyValue {
  param(
    [object] $Object,
    [string] $Name
  )

  if ($null -eq $Object) {
    return $null
  }

  $property = $Object.PSObject.Properties[$Name]
  if ($null -eq $property) {
    return $null
  }

  return $property.Value
}

function Get-CurrentCommit {
  $commit = & git rev-parse HEAD 2>$null
  if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($commit)) {
    throw "Unable to resolve the current git commit."
  }
  return $commit.Trim()
}

function Get-GitHubRepositorySlug {
  if (-not [string]::IsNullOrWhiteSpace($GitHubRepository)) {
    return $GitHubRepository.Trim()
  }

  if (-not [string]::IsNullOrWhiteSpace($env:GITHUB_REPOSITORY)) {
    return $env:GITHUB_REPOSITORY.Trim()
  }

  try {
    $remote = & git remote get-url origin 2>$null
    if ($LASTEXITCODE -eq 0 -and -not [string]::IsNullOrWhiteSpace($remote)) {
      $remote = $remote.Trim()
      if ($remote -match 'github\.com[:/](?<owner>[^/]+)/(?<repo>[^/]+?)(?:\.git)?$') {
        return "$($Matches.owner)/$($Matches.repo)"
      }
    }
  } catch {
  }

  if (Test-CommandExists gh) {
    $slug = & gh repo view --json nameWithOwner --jq .nameWithOwner 2>$null
    if ($LASTEXITCODE -eq 0 -and -not [string]::IsNullOrWhiteSpace($slug)) {
      return $slug.Trim()
    }
  }

  throw "Unable to determine the GitHub repository slug."
}

function Get-GitHubTokenValue {
  if (-not [string]::IsNullOrWhiteSpace($GitHubToken)) {
    return $GitHubToken.Trim()
  }

  if (-not [string]::IsNullOrWhiteSpace($env:GH_TOKEN)) {
    return $env:GH_TOKEN.Trim()
  }

  if (-not [string]::IsNullOrWhiteSpace($env:GITHUB_TOKEN)) {
    return $env:GITHUB_TOKEN.Trim()
  }

  return $null
}

function Get-TextWebRequest {
  param(
    [string] $Uri,
    [hashtable] $Headers = $null,
    [string] $Method = 'Get',
    [string] $Body = $null,
    [string] $ContentType = $null
  )

  $params = @{
    Uri = $Uri
    Method = $Method
    ErrorAction = 'Stop'
    MaximumRedirection = 5
  }

  if ($Headers) {
    $params.Headers = $Headers
  }

  if ($ContentType) {
    $params.ContentType = $ContentType
  }

  if ($PSBoundParameters.ContainsKey('Body')) {
    $params.Body = $Body
  }

  $iwr = Get-Command Invoke-WebRequest
  if ($iwr.Parameters.ContainsKey('UseBasicParsing')) {
    $params.UseBasicParsing = $true
  }

  $response = Invoke-WebRequest @params
  return $response.Content
}

function Get-JsonWebRequest {
  param(
    [string] $Uri,
    [hashtable] $Headers = $null
  )

  $content = Get-TextWebRequest -Uri $Uri -Headers $Headers -Method 'Get'
  return $content | ConvertFrom-Json
}

function Download-File {
  param(
    [string] $Uri,
    [string] $OutFile,
    [hashtable] $Headers = $null
  )

  $params = @{
    Uri = $Uri
    Method = 'Get'
    OutFile = $OutFile
    ErrorAction = 'Stop'
    MaximumRedirection = 5
  }

  if ($Headers) {
    $params.Headers = $Headers
  }

  $iwr = Get-Command Invoke-WebRequest
  if ($iwr.Parameters.ContainsKey('UseBasicParsing')) {
    $params.UseBasicParsing = $true
  }

  Invoke-WebRequest @params | Out-Null
  return $OutFile
}

function Ensure-Directory {
  param([string] $Path)
  $parent = Split-Path -Parent $Path
  if (-not [string]::IsNullOrWhiteSpace($parent)) {
    New-Item -ItemType Directory -Force -Path $parent | Out-Null
  }
}

function Find-JsonStringProperty {
  param(
    [object] $Value,
    [string] $Name
  )

  if ($null -eq $Value) {
    return $null
  }

  if ($Value -is [string]) {
    return $null
  }

  if ($Value -is [System.Collections.IDictionary]) {
    foreach ($key in $Value.Keys) {
      $entry = $Value[$key]
      if ($key -eq $Name -and $entry -is [string] -and -not [string]::IsNullOrWhiteSpace($entry)) {
        return $entry
      }

      $nested = Find-JsonStringProperty -Value $entry -Name $Name
      if (-not [string]::IsNullOrWhiteSpace($nested)) {
        return $nested
      }
    }

    return $null
  }

  if ($Value -is [System.Collections.IEnumerable]) {
    foreach ($item in $Value) {
      if ($item -is [string]) {
        continue
      }

      $nested = Find-JsonStringProperty -Value $item -Name $Name
      if (-not [string]::IsNullOrWhiteSpace($nested)) {
        return $nested
      }
    }

    return $null
  }

  if ($Value.PSObject -and $Value.PSObject.Properties) {
    foreach ($property in $Value.PSObject.Properties) {
      if ($property.Name -eq $Name -and $property.Value -is [string] -and -not [string]::IsNullOrWhiteSpace($property.Value)) {
        return $property.Value
      }

      $nested = Find-JsonStringProperty -Value $property.Value -Name $Name
      if (-not [string]::IsNullOrWhiteSpace($nested)) {
        return $nested
      }
    }
  }

  return $null
}

function Get-XmlNodeValue {
  param(
    [System.Xml.XmlNode] $Node,
    [string] $Name
  )

  if ($null -eq $Node) {
    return ''
  }

  $attribute = $Node.Attributes[$Name]
  if ($attribute) {
    return $attribute.Value
  }

  $child = $Node.SelectSingleNode("./*[local-name()='$Name']")
  if ($child) {
    return $child.InnerText
  }

  return ''
}

function Try-ParseXmlFragment {
  param([string] $Fragment)

  try {
    [xml] $document = "<Root>$Fragment</Root>"
    return $document
  } catch {
    return $null
  }
}

function Test-PackageFragment {
  param([xml] $Document)

  if ($null -eq $Document) {
    return $false
  }

  $identity = $Document.SelectSingleNode("//*[local-name()='UpdateIdentity']")
  $package = $Document.SelectSingleNode("//*[local-name()='AppxMetadata']")

  return ($null -ne $identity -and $null -ne $package)
}

function Get-PackageMonikerInfo {
  param([string] $PackageMoniker)

  if ([string]::IsNullOrWhiteSpace($PackageMoniker)) {
    return $null
  }

  if ($PackageMoniker -match '^(?<Name>.+?)_(?<Version>\d+\.\d+\.\d+\.\d+)_(?<Architecture>[^_]+)__(?<FamilySuffix>.+)$') {
    return [pscustomobject]@{
      Name = $Matches.Name
      Version = $Matches.Version
      Architecture = $Matches.Architecture
      FamilySuffix = $Matches.FamilySuffix
    }
  }

  return $null
}

function Get-MsixPackageInfo {
  param(
    [string] $Path,
    [string] $ExpectedIdentityName = '',
    [string] $ExpectedArchitecture = '',
    [string] $ExpectedVersion = ''
  )

  if (-not (Test-Path -LiteralPath $Path)) {
    throw "MSIX not found: $Path"
  }

  $resolvedPath = (Resolve-Path -LiteralPath $Path).Path
  $requiredEntries = @(
    'AppxManifest.xml',
    'AppxBlockMap.xml',
    'AppxSignature.p7x',
    'AppxMetadata/CodeIntegrity.cat'
  )

  $archive = [System.IO.Compression.ZipFile]::OpenRead($resolvedPath)
  try {
    foreach ($entryName in $requiredEntries) {
      if ($null -eq $archive.GetEntry($entryName)) {
        throw "The package is missing required entry: $entryName"
      }
    }

    $manifestEntry = $archive.GetEntry('AppxManifest.xml')
    if ($null -eq $manifestEntry) {
      throw 'The package does not contain AppxManifest.xml.'
    }

    $reader = New-Object System.IO.StreamReader($manifestEntry.Open())
    try {
      [xml] $manifestXml = $reader.ReadToEnd()
    } finally {
      $reader.Dispose()
    }
  } finally {
    $archive.Dispose()
  }

  $identityNode = $manifestXml.Package.Identity
  if ($null -eq $identityNode) {
    throw 'AppxManifest.xml does not contain a Package/Identity node.'
  }

  $identityName = $identityNode.Name
  $identityVersion = $identityNode.Version
  $publisher = $identityNode.Publisher
  $processorArchitecture = $identityNode.ProcessorArchitecture

  if (-not [string]::IsNullOrWhiteSpace($ExpectedIdentityName) -and $identityName -ne $ExpectedIdentityName) {
    throw "Unexpected package identity. Expected $ExpectedIdentityName, got $identityName."
  }

  if (-not [string]::IsNullOrWhiteSpace($ExpectedVersion) -and $identityVersion -ne $ExpectedVersion) {
    throw "Unexpected package version. Expected $ExpectedVersion, got $identityVersion."
  }

  if (-not [string]::IsNullOrWhiteSpace($ExpectedArchitecture) -and $processorArchitecture -and ($processorArchitecture.ToString().ToLowerInvariant() -ne $ExpectedArchitecture.ToLowerInvariant())) {
    throw "Unexpected processor architecture. Expected $ExpectedArchitecture, got $processorArchitecture."
  }

  $packageMoniker = [System.IO.Path]::GetFileNameWithoutExtension($resolvedPath)
  $monikerInfo = Get-PackageMonikerInfo -PackageMoniker $packageMoniker

  if ($monikerInfo) {
    if ($monikerInfo.Name -and $monikerInfo.Name -ne $identityName) {
      throw "Package moniker identity does not match AppxManifest identity. Moniker: $($monikerInfo.Name), manifest: $identityName."
    }

    if ($monikerInfo.Version -and $monikerInfo.Version -ne $identityVersion) {
      throw "Package moniker version does not match AppxManifest version. Moniker: $($monikerInfo.Version), manifest: $identityVersion."
    }
  }

  $packageFamilyName = $null
  if ($monikerInfo -and -not [string]::IsNullOrWhiteSpace($monikerInfo.FamilySuffix)) {
    $packageFamilyName = "$identityName`_$($monikerInfo.FamilySuffix)"
  }

  $hash = (Get-FileHash -Algorithm SHA256 -Path $resolvedPath).Hash.ToLowerInvariant()
  $size = (Get-Item -LiteralPath $resolvedPath).Length

  return [pscustomobject]@{
    SourceKind = 'LocalMsix'
    SourcePath = $resolvedPath
    FileName = [System.IO.Path]::GetFileName($resolvedPath)
    PackageMoniker = $packageMoniker
    PackageFamilyName = $packageFamilyName
    IdentityName = $identityName
    IdentityVersion = $identityVersion
    Publisher = $publisher
    ProcessorArchitecture = $processorArchitecture
    Sha256 = $hash
    Size = $size
    RequiredEntries = $requiredEntries
    ManifestXml = $manifestXml
  }
}

function Resolve-ManifestDescriptor {
  param([object] $Manifest)

  $downloadUrl = Find-JsonStringProperty -Value $Manifest -Name 'downloadUrl'
  if ([string]::IsNullOrWhiteSpace($downloadUrl)) {
    $downloadUrl = Find-JsonStringProperty -Value $Manifest -Name 'msixUrl'
  }
  if ([string]::IsNullOrWhiteSpace($downloadUrl)) {
    $downloadUrl = Find-JsonStringProperty -Value $Manifest -Name 'packageUrl'
  }
  if ([string]::IsNullOrWhiteSpace($downloadUrl)) {
    $downloadUrl = Find-JsonStringProperty -Value $Manifest -Name 'url'
  }

  $appInstallerUrl = Find-JsonStringProperty -Value $Manifest -Name 'appInstallerUrl'
  if ([string]::IsNullOrWhiteSpace($appInstallerUrl)) {
    $appInstallerUrl = Find-JsonStringProperty -Value $Manifest -Name 'installerUrl'
  }
  if ([string]::IsNullOrWhiteSpace($appInstallerUrl)) {
    $appInstallerUrl = Find-JsonStringProperty -Value $Manifest -Name 'appinstallerUrl'
  }

  $blobFeedUrl = Find-JsonStringProperty -Value $Manifest -Name 'blobFeedUrl'
  if ([string]::IsNullOrWhiteSpace($blobFeedUrl)) {
    $blobFeedUrl = Find-JsonStringProperty -Value $Manifest -Name 'blobUrl'
  }
  if ([string]::IsNullOrWhiteSpace($blobFeedUrl)) {
    $blobFeedUrl = Find-JsonStringProperty -Value $Manifest -Name 'feedUrl'
  }

  $storeProductId = Find-JsonStringProperty -Value $Manifest -Name 'storeProductId'
  if ([string]::IsNullOrWhiteSpace($storeProductId)) {
    $storeProductId = Find-JsonStringProperty -Value $Manifest -Name 'productId'
  }

  $packageIdentity = Find-JsonStringProperty -Value $Manifest -Name 'packageIdentity'
  if ([string]::IsNullOrWhiteSpace($packageIdentity)) {
    $packageIdentity = Find-JsonStringProperty -Value $Manifest -Name 'packageIdentityName'
  }

  $buildVersion = Find-JsonStringProperty -Value $Manifest -Name 'buildVersion'
  if ([string]::IsNullOrWhiteSpace($buildVersion)) {
    $buildVersion = Find-JsonStringProperty -Value $Manifest -Name 'version'
  }

  if (-not [string]::IsNullOrWhiteSpace($downloadUrl)) {
    return [pscustomobject]@{
      Kind = 'DirectMsix'
      Url = $downloadUrl
      ExpectedVersion = $buildVersion
      PackageIdentity = $packageIdentity
      StoreProductId = $storeProductId
    }
  }

  if (-not [string]::IsNullOrWhiteSpace($appInstallerUrl)) {
    return [pscustomobject]@{
      Kind = 'AppInstaller'
      Url = $appInstallerUrl
      ExpectedVersion = $buildVersion
      PackageIdentity = $packageIdentity
      StoreProductId = $storeProductId
    }
  }

  if (-not [string]::IsNullOrWhiteSpace($blobFeedUrl)) {
    return [pscustomobject]@{
      Kind = 'BlobFeed'
      Url = $blobFeedUrl
      ExpectedVersion = $buildVersion
      PackageIdentity = $packageIdentity
      StoreProductId = $storeProductId
    }
  }

  if (-not [string]::IsNullOrWhiteSpace($storeProductId) -and -not [string]::IsNullOrWhiteSpace($packageIdentity)) {
    return [pscustomobject]@{
      Kind = 'Store'
      ProductId = $storeProductId
      PackageIdentity = $packageIdentity
      ExpectedVersion = $buildVersion
    }
  }

  throw 'The upstream feed shape is not recognized.'
}

function Resolve-DescriptorFromContent {
  param(
    [string] $Content,
    [string] $SourceUrl
  )

  $trimmed = $Content.TrimStart()
  if ($trimmed.StartsWith('{') -or $trimmed.StartsWith('[')) {
    $manifest = $Content | ConvertFrom-Json
    return (Resolve-ManifestDescriptor -Manifest $manifest)
  }

  if ($trimmed.StartsWith('<')) {
    [xml] $xml = $Content
    $packageUrl = Get-FirstXmlText -Document $xml -Names @('MainPackageUri', 'PackageUri', 'Uri', 'Url', 'DownloadUrl')
    if (-not [string]::IsNullOrWhiteSpace($packageUrl)) {
      if ($packageUrl -match '\.appinstaller(\?|$)') {
        return [pscustomobject]@{
          Kind = 'AppInstaller'
          Url = $packageUrl
          SourceUrl = $SourceUrl
        }
      }

      return [pscustomobject]@{
        Kind = 'DirectMsix'
        Url = $packageUrl
        SourceUrl = $SourceUrl
      }
    }
  }

  throw "Unable to interpret feed content from $SourceUrl."
}

function Get-FirstXmlText {
  param(
    [xml] $Document,
    [string[]] $Names
  )

  foreach ($name in $Names) {
    $node = $Document.SelectSingleNode("//*[local-name()='$name']")
    if ($node) {
      $text = $node.InnerText
      if (-not [string]::IsNullOrWhiteSpace($text)) {
        return $text
      }
    }
  }

  return $null
}

function Resolve-WuCategoryId {
  param([string] $ProductId)

  $url = "https://displaycatalog.md.mp.microsoft.com/v7.0/products/$([Uri]::EscapeDataString($ProductId))?market=US&languages=en-US,en,neutral"
  $json = Get-JsonWebRequest -Uri $url
  $wuCategoryId = Find-JsonStringProperty -Value $json -Name 'WuCategoryId'
  if ([string]::IsNullOrWhiteSpace($wuCategoryId)) {
    throw "DisplayCatalog did not return a WuCategoryId for $ProductId."
  }

  return $wuCategoryId
}

function Get-Fe3HttpClient {
  if ($script:Fe3HttpClient) {
    return $script:Fe3HttpClient
  }

  $handler = New-Object System.Net.Http.HttpClientHandler
  $handler.AutomaticDecompression = [System.Net.DecompressionMethods]::GZip -bor [System.Net.DecompressionMethods]::Deflate

  $client = New-Object System.Net.Http.HttpClient($handler)
  $client.Timeout = [TimeSpan]::FromSeconds(120)

  $script:Fe3HttpClient = $client
  return $client
}

function Get-Fe3SecurityHeader {
  $created = [DateTimeOffset]::UtcNow
  $expires = $created.AddMinutes(5)

  return @"
<o:Security s:mustUnderstand="1" xmlns:o="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
  <Timestamp xmlns="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">
    <Created>$([string]::Format('{0:yyyy-MM-ddTHH:mm:ss.fffZ}', $created.UtcDateTime))</Created>
    <Expires>$([string]::Format('{0:yyyy-MM-ddTHH:mm:ss.fffZ}', $expires.UtcDateTime))</Expires>
  </Timestamp>
  <wuws:WindowsUpdateTicketsToken wsu:id="ClientMSA" xmlns:wuws="http://schemas.microsoft.com/msus/2014/10/WindowsUpdateAuthorization" xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">
    <TicketType Name="MSA" Version="1.0" Policy="MBI_SSL">
      <User />
    </TicketType>
  </wuws:WindowsUpdateTicketsToken>
</o:Security>
"@
}

function New-Fe3SoapEnvelope {
  param(
    [string] $Action,
    [string] $To,
    [string] $Body
  )

  $security = Get-Fe3SecurityHeader

  return @"
<s:Envelope xmlns:a="http://www.w3.org/2005/08/addressing" xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Header>
    <a:Action s:mustUnderstand="1">$Action</a:Action>
    <a:MessageID>urn:uuid:$([guid]::NewGuid())</a:MessageID>
    <a:To s:mustUnderstand="1">$To</a:To>
    $security
  </s:Header>
  <s:Body>
    $Body
  </s:Body>
</s:Envelope>
"@
}

function Post-Fe3Soap {
  param(
    [string] $Uri,
    [string] $Action,
    [string] $Body
  )

  $envelope = New-Fe3SoapEnvelope -Action $Action -To $Uri -Body $Body
  $client = Get-Fe3HttpClient

  $request = New-Object System.Net.Http.HttpRequestMessage([System.Net.Http.HttpMethod]::Post, $Uri)
  $request.Headers.TryAddWithoutValidation('User-Agent', 'codex-unpacked/1.0') | Out-Null
  $request.Headers.TryAddWithoutValidation('MS-CV', ([guid]::NewGuid().ToString('N').Substring(0,16) + '.0')) | Out-Null
  $request.Content = New-Object System.Net.Http.StringContent($envelope, [System.Text.Encoding]::UTF8, 'application/soap+xml')

  $response = $client.SendAsync($request).GetAwaiter().GetResult()
  $content = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()

  if (-not $response.IsSuccessStatusCode) {
    throw "The remote server returned an error: ([int]$($response.StatusCode)) $($response.ReasonPhrase)`n$content"
  }

  return $content
}

function Get-Fe3Cookie {
  $uri = 'https://fe3.delivery.mp.microsoft.com/ClientWebService/client.asmx'
  $body = @"
<GetCookie xmlns="http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService">
  <oldCookie></oldCookie>
  <lastChange>2015-10-21T17:01:07.1472913Z</lastChange>
  <currentTime>$([DateTimeOffset]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ss.fffZ'))</currentTime>
  <protocolVersion>1.40</protocolVersion>
</GetCookie>
"@

  $content = Post-Fe3Soap -Uri $uri -Action 'http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService/GetCookie' -Body $body
  [xml] $document = $content
  $encryptedData = Get-FirstXmlText -Document $document -Names @('EncryptedData')
  if ([string]::IsNullOrWhiteSpace($encryptedData)) {
    throw 'FE3 GetCookie did not return EncryptedData.'
  }

  return $encryptedData
}

function Get-Fe3DeviceAttributes {
  return 'OSArchitecture=AMD64;DeviceFamily=Windows.Desktop;App=WU;AppVer=10.0.22621.1;OSVersion=10.0.22621.1;InstallationType=Client;IsDeviceRetailDemo=0;'
}

function Get-Fe3InstalledUpdateIds {
  return @(
    1,2,3,11,19,544,549,2359974,5169044,8788830,23110993,23110994,
    54341900,54343656,59830006,59830007,59830008,60484010,62450018,62450019,
    62450020,66027979,66053150,97657898,98822896,98959022,98959023,98959024,
    98959025,98959026,104433538,104900364,105489019,117765322,129905029,
    130040031,132387090,132393049,138537048,140377312,143747671,158941041,
    158941042,158941043,158941044,159123858,159130928,164836897,164847386,
    164848327,164852241,164852246,164852252,164852253
  )
}

function Sync-Fe3Updates {
  param([string] $WuCategoryId)

  $installedIds = (Get-Fe3InstalledUpdateIds | ForEach-Object { "<int>$_</int>" }) -join ''
  $body = @"
<SyncUpdates xmlns="http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService">
  <cookie>
    <Expiration>$([DateTimeOffset]::UtcNow.AddDays(1).ToString('yyyy-MM-ddTHH:mm:ss.fffZ'))</Expiration>
    <EncryptedData>$([System.Security.SecurityElement]::Escape((Get-Fe3Cookie)))</EncryptedData>
  </cookie>
  <parameters>
    <ExpressQuery>false</ExpressQuery>
    <InstalledNonLeafUpdateIDs>$installedIds</InstalledNonLeafUpdateIDs>
    <OtherCachedUpdateIDs></OtherCachedUpdateIDs>
    <SkipSoftwareSync>false</SkipSoftwareSync>
    <NeedTwoGroupOutOfScopeUpdates>true</NeedTwoGroupOutOfScopeUpdates>
    <FilterAppCategoryIds>
      <CategoryIdentifier>
        <Id>$([System.Security.SecurityElement]::Escape($WuCategoryId))</Id>
      </CategoryIdentifier>
    </FilterAppCategoryIds>
    <TreatAppCategoryIdsAsInstalled>true</TreatAppCategoryIdsAsInstalled>
    <AlsoPerformRegularSync>false</AlsoPerformRegularSync>
    <ComputerSpec />
    <ExtendedUpdateInfoParameters>
      <XmlUpdateFragmentTypes>
        <XmlUpdateFragmentType>Extended</XmlUpdateFragmentType>
      </XmlUpdateFragmentTypes>
      <Locales>
        <string>en-US</string>
        <string>en</string>
      </Locales>
    </ExtendedUpdateInfoParameters>
    <ClientPreferredLanguages>
      <string>en-US</string>
    </ClientPreferredLanguages>
    <ProductsParameters>
      <SyncCurrentVersionOnly>false</SyncCurrentVersionOnly>
      <DeviceAttributes>$(Get-Fe3DeviceAttributes)</DeviceAttributes>
      <CallerAttributes>Interactive=1;IsSeeker=0;</CallerAttributes>
      <Products />
    </ProductsParameters>
  </parameters>
</SyncUpdates>
"@

  return Post-Fe3Soap -Uri 'https://fe3.delivery.mp.microsoft.com/ClientWebService/client.asmx' -Action 'http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService/SyncUpdates' -Body $body
}

function Parse-PackageCandidates {
  param([string] $SyncUpdatesXml)

  [xml] $document = $SyncUpdatesXml
  $nodes = $document.SelectNodes("//*[local-name()='Xml']")
  $candidates = New-Object System.Collections.Generic.List[object]

  foreach ($node in $nodes) {
    $fragment = $node.InnerText
    $fragmentDocument = Try-ParseXmlFragment -Fragment $fragment
    if (-not (Test-PackageFragment -Document $fragmentDocument)) {
      $decodedFragment = [System.Net.WebUtility]::HtmlDecode($fragment)
      if ($decodedFragment -ne $fragment) {
        $fragmentDocument = Try-ParseXmlFragment -Fragment $decodedFragment
      }
    }

    if (-not (Test-PackageFragment -Document $fragmentDocument)) {
      continue
    }

    $identityElement = $fragmentDocument.SelectSingleNode("//*[local-name()='UpdateIdentity']")
    $packageElement = $fragmentDocument.SelectSingleNode("//*[local-name()='AppxMetadata']")
    $updateId = Get-XmlNodeValue -Node $identityElement -Name 'UpdateID'
    $revisionNumber = Get-XmlNodeValue -Node $identityElement -Name 'RevisionNumber'
    $packageMoniker = Get-XmlNodeValue -Node $packageElement -Name 'PackageMoniker'
    $packageType = Get-XmlNodeValue -Node $packageElement -Name 'PackageType'

    if ([string]::IsNullOrWhiteSpace($updateId) -or [string]::IsNullOrWhiteSpace($revisionNumber) -or [string]::IsNullOrWhiteSpace($packageMoniker) -or [string]::IsNullOrWhiteSpace($packageType)) {
      continue
    }

    $monikerInfo = Get-PackageMonikerInfo -PackageMoniker $packageMoniker
    $version = [version]'0.0.0.0'
    if ($monikerInfo -and -not [string]::IsNullOrWhiteSpace($monikerInfo.Version)) {
      [version]::TryParse($monikerInfo.Version, [ref] $version) | Out-Null
    }

    $candidates.Add([pscustomobject]@{
      PackageMoniker = $packageMoniker
      PackageType = $packageType
      UpdateId = $updateId
      RevisionNumber = $revisionNumber
      Version = $version
    })
  }

  return $candidates
}

function Get-Fe3PackageUrl {
  param(
    [string] $UpdateId,
    [string] $RevisionNumber
  )

  $body = @"
<GetExtendedUpdateInfo2 xmlns="http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService">
  <updateIDs>
    <UpdateIdentity>
      <UpdateID>$([System.Security.SecurityElement]::Escape($UpdateId))</UpdateID>
      <RevisionNumber>$([System.Security.SecurityElement]::Escape($RevisionNumber))</RevisionNumber>
    </UpdateIdentity>
  </updateIDs>
  <infoTypes>
    <XmlUpdateFragmentType>FileUrl</XmlUpdateFragmentType>
    <XmlUpdateFragmentType>FileDecryption</XmlUpdateFragmentType>
  </infoTypes>
  <deviceAttributes>$(Get-Fe3DeviceAttributes)</deviceAttributes>
</GetExtendedUpdateInfo2>
"@

  $content = Post-Fe3Soap -Uri 'https://fe3.delivery.mp.microsoft.com/ClientWebService/client.asmx/secure' -Action 'http://www.microsoft.com/SoftwareDistribution/Server/ClientWebService/GetExtendedUpdateInfo2' -Body $body
  [xml] $document = $content

  $urls = $document.SelectNodes("//*[local-name()='Url']") | ForEach-Object {
    $candidate = $_.InnerText
    if ([System.Uri]::TryCreate($candidate, [System.UriKind]::Absolute, [ref] $null)) {
      return [System.Uri] $candidate
    }
  } | Where-Object {
    $_ -and ($_.Scheme -eq 'http' -or $_.Scheme -eq 'https')
  } | Where-Object {
    $_.Host -eq 'dl.delivery.mp.microsoft.com' -or $_.Host.EndsWith('.dl.delivery.mp.microsoft.com', [System.StringComparison]::OrdinalIgnoreCase)
  } | Sort-Object { $_.AbsoluteUri.Length } -Descending

  $selected = $urls | Select-Object -First 1
  if ($null -eq $selected) {
    throw "FE3 did not return a package URL for $UpdateId/$RevisionNumber."
  }

  return $selected.AbsoluteUri
}

function Resolve-StorePackageSource {
  param(
    [string] $ResolvedProductId,
    [string] $ResolvedArchitecture,
    [string] $ResolvedPackageIdentity,
    [string] $AdvertisedVersion
  )

  $wuCategoryId = Resolve-WuCategoryId -ProductId $ResolvedProductId
  $syncXml = Sync-Fe3Updates -WuCategoryId $wuCategoryId
  $candidates = Parse-PackageCandidates -SyncUpdatesXml $syncXml

  $matchingCandidates = $candidates |
    Where-Object {
      $_.PackageMoniker.StartsWith(($ResolvedPackageIdentity + '_'), [System.StringComparison]::OrdinalIgnoreCase) -and
      $_.PackageMoniker.IndexOf(('_' + $ResolvedArchitecture + '__'), [System.StringComparison]::OrdinalIgnoreCase) -ge 0
    } |
    Sort-Object Version -Descending

  $package = $matchingCandidates | Select-Object -First 1
  if ($null -eq $package) {
    $candidateSummary = ($candidates | Sort-Object PackageMoniker | ForEach-Object {
      "$($_.PackageType)`t$($_.PackageMoniker)`t$($_.UpdateId)/$($_.RevisionNumber)"
    }) -join [Environment]::NewLine
    throw "No matching package was found for $ResolvedProductId / $ResolvedArchitecture.`n$candidateSummary"
  }

  $downloadUrl = Get-Fe3PackageUrl -UpdateId $package.UpdateId -RevisionNumber $package.RevisionNumber
  $currentVersion = $package.Version.ToString()

  return [pscustomobject]@{
    Kind = 'Store'
    ProductId = $ResolvedProductId
    Architecture = $ResolvedArchitecture
    PackageIdentity = $ResolvedPackageIdentity
    AdvertisedVersion = $AdvertisedVersion
    WuCategoryId = $wuCategoryId
    PackageMoniker = $package.PackageMoniker
    PackageType = $package.PackageType
    UpdateId = $package.UpdateId
    RevisionNumber = $package.RevisionNumber
    DownloadUrl = $downloadUrl
    PackageVersion = $currentVersion
  }
}

function Resolve-MirrorReleaseSource {
  param(
    [string] $ResolvedArchitecture,
    [string] $ResolvedPackageIdentity
  )

  $release = Get-JsonWebRequest -Uri 'https://api.github.com/repos/Wangnov/codex-app-mirror/releases/latest' -Headers @{
    'User-Agent' = 'codex-unpacked/1.0'
    'Accept' = 'application/vnd.github+json'
  }

  $assets = $release.assets
  if ($null -eq $assets) {
    throw 'The mirror release did not contain any assets.'
  }

  $manifestAsset = $assets | Where-Object { $_.name -eq 'release-manifest.json' } | Select-Object -First 1
  $checksumAsset = $assets | Where-Object { $_.name -eq 'SHA256SUMS-windows.txt' } | Select-Object -First 1
  $msixAsset = $assets | Where-Object {
    $_.name -match '^OpenAI\.Codex_.+_x64__2p2nqsd0c76g0\.Msix$'
  } | Select-Object -First 1

  if ($null -eq $manifestAsset -or $null -eq $msixAsset) {
    throw 'The mirror release is missing the Windows manifest or MSIX asset.'
  }

  $tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ('codex-mirror-' + [guid]::NewGuid().ToString('N'))
  New-Item -ItemType Directory -Force -Path $tempDir | Out-Null

  try {
    $manifestPath = Join-Path $tempDir 'release-manifest.json'
    Download-File -Uri $manifestAsset.browser_download_url -OutFile $manifestPath -Headers @{ 'User-Agent' = 'codex-unpacked/1.0' } | Out-Null
    $manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json

    $packageMoniker = $manifest.sources.windows.packageMoniker
    $packageVersion = $manifest.sources.windows.version
    $advertisedVersion = $manifest.sources.windows.updateManifest.buildVersion
    $downloadUrl = $msixAsset.browser_download_url

    $checksum = $null
    if ($checksumAsset) {
      $checksumPath = Join-Path $tempDir 'SHA256SUMS-windows.txt'
      Download-File -Uri $checksumAsset.browser_download_url -OutFile $checksumPath -Headers @{ 'User-Agent' = 'codex-unpacked/1.0' } | Out-Null
      $checksumLine = Get-Content -LiteralPath $checksumPath -Raw
      if ($checksumLine -match '^(?<hash>[0-9a-fA-F]{64})\s+(?<file>.+)$') {
        $checksum = $Matches.hash.ToLowerInvariant()
      }
    }

    return [pscustomobject]@{
      Kind = 'MirrorRelease'
      ProductId = $manifest.sources.windows.productId
      Architecture = $ResolvedArchitecture
      PackageIdentity = $ResolvedPackageIdentity
      AdvertisedVersion = $advertisedVersion
      PackageVersion = $packageVersion
      PackageMoniker = $packageMoniker
      DownloadUrl = $downloadUrl
      MirrorReleaseTag = $release.tag_name
      MirrorReleaseUrl = $release.html_url
      SourceFeedUrl = $release.html_url
      UpdateManifestUrl = $manifest.sources.windows.updateManifestUrl
      ExpectedSha256 = $checksum
    }
  } finally {
    if (Test-Path -LiteralPath $tempDir) {
      Remove-Item -LiteralPath $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
  }
}

function Resolve-UpstreamSource {
  param(
    [string] $ManifestUrl,
    [string] $ResolvedProductId,
    [string] $ResolvedArchitecture,
    [string] $ResolvedPackageIdentity
  )

  $manifest = Get-JsonWebRequest -Uri $ManifestUrl
  $descriptor = Resolve-ManifestDescriptor -Manifest $manifest

  switch ($descriptor.Kind) {
    'DirectMsix' {
      return [pscustomobject]@{
        Kind = 'DirectMsix'
        DownloadUrl = $descriptor.Url
        ExpectedVersion = $descriptor.ExpectedVersion
        PackageIdentity = $descriptor.PackageIdentity
        StoreProductId = $descriptor.StoreProductId
        UpdateManifest = $manifest
        UpdateManifestUrl = $ManifestUrl
      }
    }
    'AppInstaller' {
      $content = Get-TextWebRequest -Uri $descriptor.Url -Headers @{ 'User-Agent' = 'codex-unpacked/1.0' }
      $nextDescriptor = Resolve-DescriptorFromContent -Content $content -SourceUrl $descriptor.Url
      if ($nextDescriptor.Kind -eq 'DirectMsix') {
        return [pscustomobject]@{
          Kind = 'DirectMsix'
          DownloadUrl = $nextDescriptor.Url
          ExpectedVersion = $descriptor.ExpectedVersion
          PackageIdentity = $descriptor.PackageIdentity
          StoreProductId = $descriptor.StoreProductId
          UpdateManifest = $manifest
          UpdateManifestUrl = $ManifestUrl
          SourceFeedUrl = $descriptor.Url
        }
      }

      return Resolve-UpstreamSource -ManifestUrl $nextDescriptor.Url -ResolvedProductId $ResolvedProductId -ResolvedArchitecture $ResolvedArchitecture -ResolvedPackageIdentity $ResolvedPackageIdentity
    }
    'BlobFeed' {
      $content = Get-TextWebRequest -Uri $descriptor.Url -Headers @{ 'User-Agent' = 'codex-unpacked/1.0' }
      $nextDescriptor = Resolve-DescriptorFromContent -Content $content -SourceUrl $descriptor.Url
      if ($nextDescriptor.Kind -eq 'DirectMsix') {
        return [pscustomobject]@{
          Kind = 'DirectMsix'
          DownloadUrl = $nextDescriptor.Url
          ExpectedVersion = $descriptor.ExpectedVersion
          PackageIdentity = $descriptor.PackageIdentity
          StoreProductId = $descriptor.StoreProductId
          UpdateManifest = $manifest
          UpdateManifestUrl = $ManifestUrl
          SourceFeedUrl = $descriptor.Url
        }
      }

      return Resolve-UpstreamSource -ManifestUrl $nextDescriptor.Url -ResolvedProductId $ResolvedProductId -ResolvedArchitecture $ResolvedArchitecture -ResolvedPackageIdentity $ResolvedPackageIdentity
    }
    'Store' {
      return (Resolve-StorePackageSource -ResolvedProductId $descriptor.ProductId -ResolvedArchitecture $ResolvedArchitecture -ResolvedPackageIdentity $ResolvedPackageIdentity -AdvertisedVersion $descriptor.ExpectedVersion)
    }
    default {
      throw "Unsupported upstream source kind: $($descriptor.Kind)"
    }
  }
}

function Test-PackageStateMatches {
  param(
    [object] $State,
    [object] $PackageInfo
  )

  if ($null -eq $State -or $null -eq $State.package) {
    return $false
  }

  $stateVersion = $State.package.version
  $stateSha = $State.package.sha256

  if ([string]::IsNullOrWhiteSpace($stateVersion) -or [string]::IsNullOrWhiteSpace($stateSha)) {
    return $false
  }

  return ($stateVersion -eq $PackageInfo.IdentityVersion -and $stateSha.ToLowerInvariant() -eq $PackageInfo.Sha256.ToLowerInvariant())
}

function New-ReleaseTag {
  param(
    [string] $Prefix,
    [string] $Version,
    [string] $Sha256,
    [switch] $ForceRelease
  )

  $shortHash = $Sha256.Substring(0, 12).ToLowerInvariant()
  $tag = "$Prefix-$Version-$shortHash"
  if ($ForceRelease) {
    $tag = "$tag-force-$([DateTime]::UtcNow.ToString('yyyyMMddHHmmss'))"
  }

  return $tag
}

function New-ReleaseNotes {
  param(
    [object] $PackageInfo,
    [object] $SourceInfo
  )

  $sourceUrl = $SourceInfo.DownloadUrl
  if ([string]::IsNullOrWhiteSpace($sourceUrl) -and $SourceInfo.SourceFeedUrl) {
    $sourceUrl = $SourceInfo.SourceFeedUrl
  }

  return @"
Codex MSIX mirror update

Version: $($PackageInfo.IdentityVersion)
Package: $($PackageInfo.PackageMoniker)
SHA256: $($PackageInfo.Sha256)
Source: $sourceUrl
Resolved via: $($SourceInfo.Kind)
Advertised build: $($SourceInfo.AdvertisedVersion)
Validated inputs: AppxManifest.xml, AppxBlockMap.xml, AppxSignature.p7x, AppxMetadata/CodeIntegrity.cat
Timestamp: $(Get-UtcTimestamp)
"@
}

function New-ReleaseManifest {
  param(
    [object] $PackageInfo,
    [object] $SourceInfo,
    [string] $ReleaseTag,
    [string] $ReleaseUrl = '',
    [string] $ReleaseId = ''
  )

  $sourceUrl = $SourceInfo.DownloadUrl
  if ([string]::IsNullOrWhiteSpace($sourceUrl) -and $SourceInfo.SourceFeedUrl) {
    $sourceUrl = $SourceInfo.SourceFeedUrl
  }

  return [pscustomobject]@{
    schemaVersion = 1
    generatedAt = (Get-UtcTimestamp)
    release = [pscustomobject]@{
      tag = $ReleaseTag
      id = $ReleaseId
      url = $ReleaseUrl
    }
    source = [pscustomobject]@{
      kind = $SourceInfo.Kind
      updateManifestUrl = $SourceInfo.UpdateManifestUrl
      advertisedVersion = $SourceInfo.AdvertisedVersion
      productId = $SourceInfo.ProductId
      architecture = $SourceInfo.Architecture
      packageIdentity = $SourceInfo.PackageIdentity
      packageMoniker = $SourceInfo.PackageMoniker
      downloadUrl = $sourceUrl
      wuCategoryId = $SourceInfo.WuCategoryId
      updateId = $SourceInfo.UpdateId
      revisionNumber = $SourceInfo.RevisionNumber
    }
    package = [pscustomobject]@{
      name = $PackageInfo.IdentityName
      version = $PackageInfo.IdentityVersion
      packageMoniker = $PackageInfo.PackageMoniker
      packageFamilyName = $PackageInfo.PackageFamilyName
      publisher = $PackageInfo.Publisher
      processorArchitecture = $PackageInfo.ProcessorArchitecture
      sha256 = $PackageInfo.Sha256
      size = $PackageInfo.Size
      fileName = $PackageInfo.FileName
      sourcePath = $PackageInfo.SourcePath
    }
  }
}

function Write-JsonFile {
  param(
    [string] $Path,
    [object] $Object
  )

  Ensure-Directory -Path $Path
  $Object | ConvertTo-Json -Depth 32 | Set-Content -LiteralPath $Path -Encoding utf8
}

function Read-JsonFile {
  param([string] $Path)

  if (-not (Test-Path -LiteralPath $Path)) {
    return $null
  }

  $content = Get-Content -LiteralPath $Path -Raw
  if ([string]::IsNullOrWhiteSpace($content)) {
    return $null
  }

  return $content | ConvertFrom-Json
}

function Publish-WithGh {
  param(
    [string] $RepoSlug,
    [string] $CommitSha,
    [string] $ReleaseTag,
    [string] $Title,
    [string] $NotesPath,
    [string[]] $Assets
  )

  $args = @(
    'release', 'create', $ReleaseTag,
    '--repo', $RepoSlug,
    '--target', $CommitSha,
    '--title', $Title,
    '--notes-file', $NotesPath
  )

  foreach ($asset in $Assets) {
    $args += $asset
  }

  & gh @args | Out-Host
  if ($LASTEXITCODE -ne 0) {
    throw 'gh release create failed.'
  }

  $releaseJson = & gh release view $ReleaseTag --repo $RepoSlug --json id,url,tagName,name,publishedAt
  if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($releaseJson)) {
    throw 'gh release view failed after publishing.'
  }

  return $releaseJson | ConvertFrom-Json
}

function Publish-WithToken {
  param(
    [string] $RepoSlug,
    [string] $ReleaseTag,
    [string] $Title,
    [string] $Notes,
    [string[]] $Assets
  )

  $token = Get-GitHubTokenValue
  if ([string]::IsNullOrWhiteSpace($token)) {
    throw 'No GitHub token is available. Set GH_TOKEN or GITHUB_TOKEN, or install and authenticate gh.'
  }

  $headers = @{
    Authorization = "Bearer $token"
    Accept = 'application/vnd.github+json'
    'X-GitHub-Api-Version' = '2022-11-28'
  }

  $createBody = [pscustomobject]@{
    tag_name = $ReleaseTag
    name = $Title
    body = $Notes
    draft = $false
    prerelease = $false
  } | ConvertTo-Json -Depth 20

  $release = Invoke-RestMethod -Method Post -Uri "https://api.github.com/repos/$RepoSlug/releases" -Headers $headers -Body $createBody -ContentType 'application/json'

  foreach ($asset in $Assets) {
    $assetName = [System.IO.Path]::GetFileName($asset)
    $uploadUrl = $release.upload_url -replace '\{\?name,label\}', "?name=$([System.Uri]::EscapeDataString($assetName))"
    Invoke-WebRequest -Method Post -Uri $uploadUrl -Headers $headers -InFile $asset -ContentType 'application/octet-stream' | Out-Null
  }

  return $release
}

function Publish-GitHubRelease {
  param(
    [string] $RepoSlug,
    [string] $CommitSha,
    [string] $ReleaseTag,
    [string] $Title,
    [string] $Notes,
    [string] $NotesPath,
    [string[]] $Assets
  )

  if (Test-CommandExists gh) {
    & gh auth status github.com *> $null
    if ($LASTEXITCODE -eq 0) {
      return (Publish-WithGh -RepoSlug $RepoSlug -CommitSha $CommitSha -ReleaseTag $ReleaseTag -Title $Title -NotesPath $NotesPath -Assets $Assets)
    }
  }

  return (Publish-WithToken -RepoSlug $RepoSlug -ReleaseTag $ReleaseTag -Title $Title -Notes $Notes -Assets $Assets)
}

function Get-DownloadTargetPath {
  param(
    [string] $BaseDirectory,
    [string] $PackageMoniker
  )

  Ensure-Directory -Path (Join-Path $BaseDirectory 'placeholder.txt')
  return (Join-Path $BaseDirectory ($PackageMoniker + '.Msix'))
}

function New-StateObject {
  param(
    [object] $PackageInfo,
    [object] $SourceInfo,
    [string] $ReleaseTag,
    [string] $ReleaseId = '',
    [string] $ReleaseUrl = ''
  )

  $sourceUrl = $SourceInfo.DownloadUrl
  if ([string]::IsNullOrWhiteSpace($sourceUrl) -and $SourceInfo.SourceFeedUrl) {
    $sourceUrl = $SourceInfo.SourceFeedUrl
  }

  return [pscustomobject]@{
    schemaVersion = 1
    updatedAt = (Get-UtcTimestamp)
    package = [pscustomobject]@{
      name = $PackageInfo.IdentityName
      version = $PackageInfo.IdentityVersion
      packageMoniker = $PackageInfo.PackageMoniker
      packageFamilyName = $PackageInfo.PackageFamilyName
      publisher = $PackageInfo.Publisher
      sha256 = $PackageInfo.Sha256
      size = $PackageInfo.Size
      fileName = $PackageInfo.FileName
      sourceKind = $PackageInfo.SourceKind
      sourcePath = $PackageInfo.SourcePath
    }
    source = [pscustomobject]@{
      kind = $SourceInfo.Kind
      updateManifestUrl = $SourceInfo.UpdateManifestUrl
      advertisedVersion = $SourceInfo.AdvertisedVersion
      productId = $SourceInfo.ProductId
      architecture = $SourceInfo.Architecture
      packageIdentity = $SourceInfo.PackageIdentity
      packageMoniker = $SourceInfo.PackageMoniker
      downloadUrl = $sourceUrl
      wuCategoryId = $SourceInfo.WuCategoryId
      updateId = $SourceInfo.UpdateId
      revisionNumber = $SourceInfo.RevisionNumber
    }
    release = [pscustomobject]@{
      tag = $ReleaseTag
      id = $ReleaseId
      url = $ReleaseUrl
    }
  }
}

function Get-ResolvedSourceSummary {
  param([object] $SourceInfo)

  $summary = @(
    "kind=$($SourceInfo.Kind)"
    "version=$($SourceInfo.PackageVersion)"
    "moniker=$($SourceInfo.PackageMoniker)"
    "downloadUrl=$($SourceInfo.DownloadUrl)"
    "advertised=$($SourceInfo.AdvertisedVersion)"
  ) -join '; '

  return $summary
}

function Resolve-LiveCandidate {
  param(
    [string] $ManifestUrl,
    [string] $ResolvedProductId,
    [string] $ResolvedArchitecture,
    [string] $ResolvedPackageIdentity
  )

  $manifest = Get-JsonWebRequest -Uri $ManifestUrl

  try {
    $resolved = Resolve-UpstreamSource -ManifestUrl $ManifestUrl -ResolvedProductId $ResolvedProductId -ResolvedArchitecture $ResolvedArchitecture -ResolvedPackageIdentity $ResolvedPackageIdentity
    $resolved | Add-Member -NotePropertyName UpdateManifest -NotePropertyValue $manifest -Force
    return $resolved
  } catch {
    Write-Log -Level 'WARN' -Message "Direct Microsoft Store resolution failed: $($_.Exception.Message)"
    Write-Log -Level 'WARN' -Message 'Falling back to the public mirror release feed.'
    $mirror = Resolve-MirrorReleaseSource -ResolvedArchitecture $ResolvedArchitecture -ResolvedPackageIdentity $ResolvedPackageIdentity
    $mirror | Add-Member -NotePropertyName UpdateManifest -NotePropertyValue $manifest -Force
    return $mirror
  }
}

function Resolve-LocalCandidate {
  param([string] $Path)

  return (Get-MsixPackageInfo -Path $Path -ExpectedIdentityName $PackageIdentity -ExpectedArchitecture $Architecture)
}

function Resolve-CandidateState {
  param([object] $Candidate)

  $currentState = Read-JsonFile -Path $StatePath
  return Test-PackageStateMatches -State $currentState -PackageInfo $Candidate
}

function Write-ReleaseArtifacts {
  param(
    [string] $Directory,
    [object] $PackageInfo,
    [object] $SourceInfo,
    [string] $ReleaseTag,
    [object] $ReleaseRecord
  )

  $packageTarget = Join-Path $Directory $PackageInfo.FileName
  Copy-Item -LiteralPath $PackageInfo.SourcePath -Destination $packageTarget -Force

  $shaPath = Join-Path $Directory 'SHA256SUMS.txt'
  "$($PackageInfo.Sha256)  $($PackageInfo.FileName)" | Set-Content -LiteralPath $shaPath -Encoding ascii

  $releaseJsonPath = Join-Path $Directory 'release.json'
  $releaseManifest = New-ReleaseManifest -PackageInfo $PackageInfo -SourceInfo $SourceInfo -ReleaseTag $ReleaseTag -ReleaseUrl $ReleaseRecord.url -ReleaseId $ReleaseRecord.id
  Write-JsonFile -Path $releaseJsonPath -Object $releaseManifest

  $notesPath = Join-Path $Directory 'release-notes.md'
  New-ReleaseNotes -PackageInfo $PackageInfo -SourceInfo $SourceInfo | Set-Content -LiteralPath $notesPath -Encoding utf8

  return [pscustomobject]@{
    Package = $packageTarget
    Sha = $shaPath
    ReleaseJson = $releaseJsonPath
    Notes = $notesPath
  }
}

function Main {
  $tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ('codex-unpacked-' + [guid]::NewGuid().ToString('N'))
  $packageInfo = $null
  $sourceInfo = $null
  $resolved = $null
  $probeOnlyMode = [bool]$ProbeOnly
  $dryRunMode = [bool]$DryRun
  $localMode = -not [string]::IsNullOrWhiteSpace($LocalMsixPath)

  try {
    if ($localMode) {
      Write-Log -Level 'INFO' -Message "Inspecting local package: $LocalMsixPath"
      $packageInfo = Resolve-LocalCandidate -Path $LocalMsixPath
      $sourceInfo = [pscustomobject]@{
        Kind = 'LocalMsix'
        DownloadUrl = $null
        SourceFeedUrl = $null
        UpdateManifestUrl = $null
        AdvertisedVersion = $packageInfo.IdentityVersion
        PackageIdentity = $packageInfo.IdentityName
        PackageMoniker = $packageInfo.PackageMoniker
        ProductId = $ProductId
        Architecture = $Architecture
        WuCategoryId = $null
        UpdateId = $null
        RevisionNumber = $null
        PackageVersion = $packageInfo.IdentityVersion
      }
    } else {
      Write-Log -Level 'INFO' -Message "Resolving upstream package from $UpdateManifestUrl"
      $resolved = Resolve-LiveCandidate -ManifestUrl $UpdateManifestUrl -ResolvedProductId $ProductId -ResolvedArchitecture $Architecture -ResolvedPackageIdentity $PackageIdentity
      $sourceInfo = [pscustomobject]@{
        Kind = Get-ObjectPropertyValue -Object $resolved -Name 'Kind'
        DownloadUrl = Get-ObjectPropertyValue -Object $resolved -Name 'DownloadUrl'
        SourceFeedUrl = Get-ObjectPropertyValue -Object $resolved -Name 'SourceFeedUrl'
        UpdateManifestUrl = Get-ObjectPropertyValue -Object $resolved -Name 'UpdateManifestUrl'
        AdvertisedVersion = Get-ObjectPropertyValue -Object $resolved -Name 'AdvertisedVersion'
        PackageIdentity = Get-ObjectPropertyValue -Object $resolved -Name 'PackageIdentity'
        PackageMoniker = Get-ObjectPropertyValue -Object $resolved -Name 'PackageMoniker'
        ProductId = Get-ObjectPropertyValue -Object $resolved -Name 'ProductId'
        Architecture = Get-ObjectPropertyValue -Object $resolved -Name 'Architecture'
        WuCategoryId = Get-ObjectPropertyValue -Object $resolved -Name 'WuCategoryId'
        UpdateId = Get-ObjectPropertyValue -Object $resolved -Name 'UpdateId'
        RevisionNumber = Get-ObjectPropertyValue -Object $resolved -Name 'RevisionNumber'
        PackageVersion = Get-ObjectPropertyValue -Object $resolved -Name 'PackageVersion'
        MirrorReleaseTag = Get-ObjectPropertyValue -Object $resolved -Name 'MirrorReleaseTag'
        MirrorReleaseUrl = Get-ObjectPropertyValue -Object $resolved -Name 'MirrorReleaseUrl'
        ExpectedSha256 = Get-ObjectPropertyValue -Object $resolved -Name 'ExpectedSha256'
      }

      Write-Log -Level 'INFO' -Message ("Upstream source: " + (Get-ResolvedSourceSummary -SourceInfo $sourceInfo))

      if ($probeOnlyMode) {
        if (-not [string]::IsNullOrWhiteSpace($sourceInfo.AdvertisedVersion) -and -not [string]::IsNullOrWhiteSpace($sourceInfo.PackageVersion) -and $sourceInfo.AdvertisedVersion -ne $sourceInfo.PackageVersion) {
          Write-Log -Level 'WARN' -Message "Update manifest advertises $($sourceInfo.AdvertisedVersion) but FE3 resolved $($sourceInfo.PackageVersion)."
        }

        return [pscustomobject]@{
          Mode = 'ProbeOnly'
          PackageVersion = $sourceInfo.PackageVersion
          PackageMoniker = $sourceInfo.PackageMoniker
          DownloadUrl = $sourceInfo.DownloadUrl
          UpdateManifestVersion = $sourceInfo.AdvertisedVersion
        }
      }

      $downloadTarget = Get-DownloadTargetPath -BaseDirectory $tempRoot -PackageMoniker $sourceInfo.PackageMoniker
      Write-Log -Level 'INFO' -Message "Downloading package to $downloadTarget"
      Download-File -Uri $sourceInfo.DownloadUrl -OutFile $downloadTarget -Headers @{ 'User-Agent' = 'codex-unpacked/1.0' } | Out-Null
      $packageInfo = Get-MsixPackageInfo -Path $downloadTarget -ExpectedIdentityName $PackageIdentity -ExpectedArchitecture $Architecture -ExpectedVersion $sourceInfo.PackageVersion
      if (-not [string]::IsNullOrWhiteSpace($sourceInfo.ExpectedSha256) -and $packageInfo.Sha256 -ne $sourceInfo.ExpectedSha256.ToLowerInvariant()) {
        throw "Downloaded package hash mismatch. Expected $($sourceInfo.ExpectedSha256), got $($packageInfo.Sha256)."
      }
      $packageInfo | Add-Member -NotePropertyName SourceKind -NotePropertyValue $sourceInfo.Kind -Force
      $packageInfo | Add-Member -NotePropertyName DownloadUrl -NotePropertyValue $sourceInfo.DownloadUrl -Force
      $packageInfo | Add-Member -NotePropertyName AdvertisedVersion -NotePropertyValue $sourceInfo.AdvertisedVersion -Force
      $packageInfo | Add-Member -NotePropertyName PackageVersion -NotePropertyValue $sourceInfo.PackageVersion -Force
      $packageInfo | Add-Member -NotePropertyName UpdateManifestUrl -NotePropertyValue $sourceInfo.UpdateManifestUrl -Force
      $packageInfo | Add-Member -NotePropertyName UpdateId -NotePropertyValue $sourceInfo.UpdateId -Force
      $packageInfo | Add-Member -NotePropertyName RevisionNumber -NotePropertyValue $sourceInfo.RevisionNumber -Force
    }

    $stateMatches = Resolve-CandidateState -Candidate $packageInfo
    if ($stateMatches -and -not $Force) {
      Write-Log -Level 'INFO' -Message "No update: $($packageInfo.IdentityVersion) matches data/latest.json."
      return [pscustomobject]@{
        Mode = 'NoUpdate'
        Version = $packageInfo.IdentityVersion
        Sha256 = $packageInfo.Sha256
        PackageMoniker = $packageInfo.PackageMoniker
      }
    }

    if ($dryRunMode) {
      Write-Log -Level 'INFO' -Message "Dry-run: the package would be published, but no release will be created."
      return [pscustomobject]@{
        Mode = 'DryRun'
        Version = $packageInfo.IdentityVersion
        Sha256 = $packageInfo.Sha256
        PackageMoniker = $packageInfo.PackageMoniker
      }
    }

    $repoSlug = Get-GitHubRepositorySlug
    $commitSha = Get-CurrentCommit
    $releaseTag = New-ReleaseTag -Prefix $ReleaseTagPrefix -Version $packageInfo.IdentityVersion -Sha256 $packageInfo.Sha256 -ForceRelease:$Force
    $releaseTitle = "Codex MSIX $($packageInfo.IdentityVersion)"
    $notes = New-ReleaseNotes -PackageInfo $packageInfo -SourceInfo $sourceInfo

    $artifactDir = Join-Path $tempRoot 'release'
    New-Item -ItemType Directory -Force -Path $artifactDir | Out-Null
    $releaseRecord = $null

    if (-not $probeOnlyMode) {
      $packageAssetPath = if ($localMode) { (Resolve-Path -LiteralPath $LocalMsixPath).Path } else { $packageInfo.SourcePath }
      $releaseAssets = Write-ReleaseArtifacts -Directory $artifactDir -PackageInfo $packageInfo -SourceInfo $sourceInfo -ReleaseTag $releaseTag -ReleaseRecord ([pscustomobject]@{ id = ''; url = '' })
      $assets = @($releaseAssets.Package, $releaseAssets.Sha, $releaseAssets.ReleaseJson)
      $notesPath = $releaseAssets.Notes

      Write-Log -Level 'INFO' -Message "Publishing GitHub release $releaseTag to $repoSlug"
      $releaseRecord = Publish-GitHubRelease -RepoSlug $repoSlug -CommitSha $commitSha -ReleaseTag $releaseTag -Title $releaseTitle -Notes $notes -NotesPath $notesPath -Assets $assets

      $releaseManifest = New-ReleaseManifest -PackageInfo $packageInfo -SourceInfo $sourceInfo -ReleaseTag $releaseTag -ReleaseUrl $releaseRecord.url -ReleaseId $releaseRecord.id
      Write-JsonFile -Path (Join-Path $artifactDir 'release.json') -Object $releaseManifest

      $state = New-StateObject -PackageInfo $packageInfo -SourceInfo $sourceInfo -ReleaseTag $releaseTag -ReleaseId $releaseRecord.id -ReleaseUrl $releaseRecord.url
      Write-JsonFile -Path $StatePath -Object $state

      Write-Log -Level 'INFO' -Message "Published release $releaseTag"
      return [pscustomobject]@{
        Mode = 'Published'
        Version = $packageInfo.IdentityVersion
        Sha256 = $packageInfo.Sha256
        ReleaseTag = $releaseTag
        ReleaseUrl = $releaseRecord.url
        Repository = $repoSlug
      }
    }

    throw 'Unexpected execution path.'
  } finally {
    if (Test-Path -LiteralPath $tempRoot) {
      Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
  }
}

try {
  Main
} catch {
  Write-Log -Level 'ERROR' -Message $_.Exception.Message
  throw
}
