#!/usr/bin/env bash
# Compila el APK del spike S2 sin Gradle (código desechable).
set -euo pipefail
cd "$(dirname "$0")"
SDK="/c/Users/Jose/AppData/Local/Android/Sdk"
BT="$SDK/build-tools/34.0.0"
JDK="/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1"
JAR_W="$(cygpath -w "$SDK/platforms/android-30/android.jar")"
export JAVA_HOME="$(cygpath -w "$JDK")"
export PATH="$JDK/bin:$PATH"

rm -rf build && mkdir -p build/classes build/dex
"$BT/aapt2.exe" compile --dir res -o build/res.zip
"$BT/aapt2.exe" link -o build/base.apk -I "$JAR_W" --manifest AndroidManifest.xml build/res.zip
javac -source 8 -target 8 -bootclasspath "$JAR_W" -d build/classes \
  src/tech/hybriq/adbuddy/spike/a11y/ProbeService.java
"$BT/d8.bat" --release --min-api 30 --lib "$JAR_W" --output build/dex \
  $(find build/classes -name '*.class')

# Añade classes.dex sin tocar resources.arsc (debe seguir sin comprimir para targetSdk >= 30).
cp build/base.apk build/unsigned.apk
python - <<'EOF'
import zipfile
with zipfile.ZipFile("build/unsigned.apk", "a", compression=zipfile.ZIP_DEFLATED) as z:
    z.write("build/dex/classes.dex", "classes.dex")
EOF

"$BT/zipalign.exe" -f -p 4 build/unsigned.apk build/aligned.apk
[ -f spike.jks ] || keytool -genkeypair -keystore spike.jks -storepass spike123 -keypass spike123 \
  -alias spike -keyalg RSA -keysize 2048 -validity 365 -dname "CN=ADBuddy spike"
"$BT/apksigner.bat" sign --ks spike.jks --ks-pass pass:spike123 --out build/s2-a11y.apk build/aligned.apk
"$BT/apksigner.bat" verify build/s2-a11y.apk
echo "APK OK: spikes/s2-a11y/build/s2-a11y.apk"
