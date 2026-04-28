$json = Get-Content 'C:\Users\tednv\vsys\vwork\web\static\tool_leadfinder_response.json' -Raw | ConvertFrom-Json

if ($json.error) {
    Write-Host "ERROR:" $json.error.message
    exit 1
}

$parts = $json.candidates[0].content.parts
foreach ($part in $parts) {
    if ($part.inlineData) {
        Write-Host "FOUND IMAGE, mimeType:" $part.inlineData.mimeType
        Write-Host "Data length:" $part.inlineData.data.Length
        $bytes = [System.Convert]::FromBase64String($part.inlineData.data)
        [System.IO.File]::WriteAllBytes('C:\Users\tednv\vsys\vwork\web\static\tool_leadfinder.png', $bytes)
        Write-Host "Saved to tool_leadfinder.png"
    }
    elseif ($part.text) {
        Write-Host "TEXT:" $part.text
    }
}
