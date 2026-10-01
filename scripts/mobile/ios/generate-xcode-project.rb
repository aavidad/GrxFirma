#!/usr/bin/env ruby
# frozen_string_literal: true
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

require 'digest'
require 'optparse'

gem 'xcodeproj', '= 1.28.1'
require 'xcodeproj'

EXPECTED_XCODEPROJ = '1.28.1'
abort "ERROR: xcodeproj #{EXPECTED_XCODEPROJ} es obligatorio" unless Xcodeproj::VERSION == EXPECTED_XCODEPROJ

repo_root = File.expand_path('../../..', __dir__)
ios_root = File.join(repo_root, 'mobile', 'ios')
output = File.join(ios_root, 'GrxFirma.xcodeproj')
OptionParser.new do |parser|
  parser.on('--output PATH') { |path| output = File.expand_path(path) }
end.parse!

abort 'ERROR: la salida debe terminar en .xcodeproj' unless output.end_with?('.xcodeproj')

project = Xcodeproj::Project.new(output)
# xcodeproj starts with random UUIDs. Stabilize the initial graph, then assign
# deterministic creation IDs so cyclic target dependencies are reproducible.
project.predictabilize_uuids
uuid_sequence = 0
project.define_singleton_method(:generate_uuid) do
  loop do
    uuid_sequence += 1
    uuid = Digest::SHA256.hexdigest("grxfirma-ios-project-#{uuid_sequence}")[0, 24].upcase
    next if objects_by_uuid.key?(uuid) || generated_uuids.include?(uuid)

    generated_uuids << uuid
    break uuid
  end
end
project.root_object.attributes['LastSwiftUpdateCheck'] = '1600'
project.root_object.attributes['LastUpgradeCheck'] = '1600'
project.root_object.attributes['BuildIndependentTargetsInParallel'] = 'YES'
project.root_object.development_region = 'es'
project.root_object.known_regions = %w[es en Base]

groups = { '' => project.main_group }
references = {}

file_reference = lambda do |relative_path|
  references[relative_path] ||= begin
    parts = relative_path.split('/')
    file_name = parts.pop
    current_path = ''
    group = project.main_group
    parts.each do |part|
      current_path = current_path.empty? ? part : File.join(current_path, part)
      group = groups[current_path] ||= group.new_group(part, part)
    end
    group.new_file(file_name)
  end
end

debug_config = file_reference.call('Config/Debug.xcconfig')
release_config = file_reference.call('Config/Release.xcconfig')
file_reference.call('Config/Base.xcconfig')
file_reference.call('Config/Signing.example.xcconfig')

app = project.new_target(:application, 'GrxFirma', :ios, '16.0')
share = project.new_target(:app_extension, 'GrxFirmaShare', :ios, '16.0')
tests = project.new_target(:unit_test_bundle, 'GrxFirmaTests', :ios, '16.0')

common_settings = {
  'SUPPORTED_PLATFORMS' => 'iphoneos iphonesimulator',
  'TARGETED_DEVICE_FAMILY' => '1,2',
  'SUPPORTS_MACCATALYST' => 'NO',
  'ENABLE_BITCODE' => 'NO',
  'CLANG_ENABLE_MODULES' => 'YES',
  'SWIFT_VERSION' => '5.9'
}.freeze

[app, share, tests].each do |target|
  target.build_configurations.each do |configuration|
    configuration.base_configuration_reference = configuration.name == 'Release' ? release_config : debug_config
    configuration.build_settings.merge!(common_settings)
  end
end

app.build_configurations.each do |configuration|
  configuration.build_settings.merge!(
    'PRODUCT_NAME' => 'GrxFirma',
    'PRODUCT_BUNDLE_IDENTIFIER' => '$(GRXFIRMA_APP_BUNDLE_IDENTIFIER)',
    'INFOPLIST_FILE' => 'GrxFirma/Resources/Info.plist',
    'GENERATE_INFOPLIST_FILE' => 'NO',
    'CODE_SIGN_ENTITLEMENTS' => 'GrxFirma/GrxFirma.entitlements',
    'SWIFT_OBJC_BRIDGING_HEADER' => 'GrxFirma/GrxFirma-Bridging-Header.h',
    'ASSETCATALOG_COMPILER_APPICON_NAME' => 'AppIcon',
    'ASSETCATALOG_COMPILER_GLOBAL_ACCENT_COLOR_NAME' => 'AccentColor',
    'LD_RUNPATH_SEARCH_PATHS' => '$(inherited) @executable_path/Frameworks',
    'SKIP_INSTALL' => 'NO'
  )
  if configuration.name == 'Release'
    configuration.build_settings['OTHER_LDFLAGS'] = '$(inherited) -framework Mobilebind'
  end
end

share.build_configurations.each do |configuration|
  configuration.build_settings.merge!(
    'PRODUCT_NAME' => 'GrxFirmaShare',
    'PRODUCT_BUNDLE_IDENTIFIER' => '$(GRXFIRMA_SHARE_BUNDLE_IDENTIFIER)',
    'INFOPLIST_FILE' => 'ShareExtension/Info.plist',
    'GENERATE_INFOPLIST_FILE' => 'NO',
    'CODE_SIGN_ENTITLEMENTS' => 'ShareExtension/ShareExtension.entitlements',
    'APPLICATION_EXTENSION_API_ONLY' => 'YES',
    'LD_RUNPATH_SEARCH_PATHS' => '$(inherited) @executable_path/Frameworks @executable_path/../../Frameworks',
    'SKIP_INSTALL' => 'YES'
  )
  if configuration.name == 'Release'
    configuration.build_settings['PROVISIONING_PROFILE_SPECIFIER'] = '$(GRXFIRMA_SHARE_PROVISIONING_PROFILE)'
  end
end

tests.build_configurations.each do |configuration|
  configuration.build_settings.merge!(
    'PRODUCT_NAME' => 'GrxFirmaTests',
    'PRODUCT_BUNDLE_IDENTIFIER' => '$(GRXFIRMA_APP_BUNDLE_IDENTIFIER).tests',
    'GENERATE_INFOPLIST_FILE' => 'YES',
    'CODE_SIGN_ENTITLEMENTS' => '',
    'CODE_SIGN_STYLE' => 'Automatic',
    'TEST_HOST' => '$(BUILT_PRODUCTS_DIR)/GrxFirma.app/GrxFirma',
    'BUNDLE_LOADER' => '$(TEST_HOST)',
    'SKIP_INSTALL' => 'YES'
  )
end

app_sources = Dir.glob(File.join(ios_root, 'GrxFirma', '**', '*.{swift,m}'))
                 .map { |path| path.delete_prefix("#{ios_root}/") }
                 .sort
app.add_file_references(app_sources.map { |path| file_reference.call(path) })

%w[
  GrxFirma/GrxFirma-Bridging-Header.h
  GrxFirma/Core/AFV2GomobileAdapter.h
  GrxFirma/GrxFirma.entitlements
  GrxFirma/Resources/Info.plist
  GrxFirma/Resources/AppIcon.svg
].each { |path| file_reference.call(path) }

app_resources = %w[
  GrxFirma/Resources/Assets.xcassets
  GrxFirma/Resources/PrivacyInfo.xcprivacy
]
app_resources.each do |path|
  app.resources_build_phase.add_file_reference(file_reference.call(path), true)
end

share_sources = %w[
  ShareExtension/ShareViewController.swift
  GrxFirma/App/AppConfiguration.swift
  GrxFirma/Models/AppModels.swift
  GrxFirma/Documents/ShareInboxWriter.swift
]
share.add_file_references(share_sources.map { |path| file_reference.call(path) })
%w[ShareExtension/Info.plist ShareExtension/ShareExtension.entitlements].each do |path|
  file_reference.call(path)
end
share.resources_build_phase.add_file_reference(
  file_reference.call('GrxFirma/Resources/PrivacyInfo.xcprivacy'),
  true
)

test_sources = Dir.glob(File.join(ios_root, 'Tests', '*.swift'))
                  .map { |path| path.delete_prefix("#{ios_root}/") }
                  .sort
tests.add_file_references(test_sources.map { |path| file_reference.call(path) })
tests.add_dependency(app)

app.add_dependency(share)
embed_extensions = app.new_copy_files_build_phase('Embed App Extensions')
embed_extensions.symbol_dst_subfolder_spec = :plug_ins
embed_extensions.add_file_reference(share.product_reference, true)

release_gate = app.new_shell_script_build_phase('Validate Release Inputs')
release_gate.shell_path = '/bin/bash'
release_gate.shell_script = <<~'SH'
  set -euo pipefail
  if [[ "${CONFIGURATION:-}" == "Release" ]]; then
    "${SRCROOT}/../../scripts/mobile/ios/check-release-inputs.sh" --build-phase
  fi
SH
release_gate.input_paths = [
  '$(SRCROOT)/Frameworks/Mobilebind.xcframework',
  '$(SRCROOT)/Frameworks/Mobilebind.xcframework.sha256',
  '$(SRCROOT)/../../scripts/mobile/ios/check-release-inputs.sh',
  '$(SRCROOT)/../../scripts/mobile/ios/validate_core_xcframework.py'
]
app.build_phases.move(release_gate, 0)

project.sort
project.predictabilize_uuids
project.save

scheme = Xcodeproj::XCScheme.new
scheme.configure_with_targets(app, tests, launch_target: true)
scheme.add_build_target(share)
scheme.save_as(output, 'GrxFirma', true)

puts "Proyecto Xcode generado: #{output}"
