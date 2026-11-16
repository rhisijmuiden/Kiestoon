// The app runs on iPhone and Android only (src/lib.rs); this binary lets `cargo check` build it on
// a desktop too.
fn main() {
    kiestoon_mobile_lib::run()
}
