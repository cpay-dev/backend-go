fn main() -> Result<(), Box<dyn std::error::Error>> {
    tonic_build::configure()
        .out_dir("src/gen")
        .compile_protos(&["../proto/chain/v1/chain_service.proto"], &["../proto"])?;
    Ok(())
}
