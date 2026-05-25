fn main() {
    let protoc = protoc_bin_vendored::protoc_bin_path().expect("vendored protoc");
    let mut config = prost_build::Config::new();
    config.protoc_executable(protoc);
    config.type_attribute(
        ".quark.harness.v1",
        "#[derive(serde::Serialize, serde::Deserialize)] #[serde(default, rename_all = \"camelCase\")]",
    );
    config
        .compile_protos(
            &["../../proto/quark/harness/v1/harness.proto"],
            &["../../proto"],
        )
        .expect("compile harness protobuf");
    println!("cargo:rerun-if-changed=../../proto/quark/harness/v1/harness.proto");
}
