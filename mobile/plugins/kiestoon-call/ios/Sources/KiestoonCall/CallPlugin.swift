// The iPhone half of the Kiestoon app: PushKit wakes the app for a call, CallKit shows it.
//
// iOS requires that every VoIP push is reported to CallKit at once, also while the app starts in
// the background. So this plugin reports the call itself (with the caller decrypted from the push)
// and lets the page connect; the waiting call then arrives over SIP. Answer on the CallKit screen
// is passed to the page, which answers the SIP call as soon as it rings.

import AVFoundation
import CallKit
import CryptoKit
import PushKit
import ObjectiveC
import Security
import SwiftRs
import Tauri
import UIKit
import UserNotifications
import WebKit

class SecretArgs: Decodable {
  let secret: String
}

class CallUpdateArgs: Decodable {
  let state: String
  var name: String?
  var number: String?
}

// MARK: - storage

enum Keychain {
  private static let service = "nl.rhis.kiestoon.call"

  static func get(_ key: String) -> String? {
    let q: [String: Any] = [
      kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service,
      kSecAttrAccount as String: key, kSecReturnData as String: true,
    ]
    var out: AnyObject?
    guard SecItemCopyMatching(q as CFDictionary, &out) == errSecSuccess, let d = out as? Data else { return nil }
    return String(data: d, encoding: .utf8)
  }

  static func set(_ key: String, _ value: String) {
    let q: [String: Any] = [
      kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service, kSecAttrAccount as String: key,
    ]
    SecItemDelete(q as CFDictionary)
    var add = q
    add[kSecValueData as String] = Data(value.utf8)
    // A push may arrive while the phone is locked.
    add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
    SecItemAdd(add as CFDictionary, nil)
  }
}

enum Store {
  static var appId: String {
    if let id = UserDefaults.standard.string(forKey: "kt.appId") { return id }
    let id = UUID().uuidString
    UserDefaults.standard.set(id, forKey: "kt.appId")
    return id
  }

  /// Decrypts the content of a notification: AES-256-GCM, nonce || ciphertext || tag, with
  /// "kiestoon-push-data" as additional data (see server/internal/push).
  static func open(_ data: String?) -> [String: Any]? {
    guard let data = data, let secret = Keychain.get("secret"),
      let key = b64url(secret), let raw = b64url(data), key.count == 32
    else { return nil }
    do {
      let box = try AES.GCM.SealedBox(combined: raw)
      let plain = try AES.GCM.open(box, using: SymmetricKey(data: key), authenticating: Data("kiestoon-push-data".utf8))
      return try JSONSerialization.jsonObject(with: plain) as? [String: Any]
    } catch {
      return nil
    }
  }

  static func b64url(_ s: String) -> Data? {
    var t = s.replacingOccurrences(of: "-", with: "+").replacingOccurrences(of: "_", with: "/")
    while t.count % 4 != 0 { t += "=" }
    return Data(base64Encoded: t)
  }
}

// MARK: - token of visible notifications

/// iOS hands the token of visible notifications to the app delegate, which Tauri owns. This adds
/// the delegate method at run time and passes the token on as a notification.
enum AlertTokenHook {
  private static var done = false

  static func install() {
    guard !done, let delegate = UIApplication.shared.delegate else { return }
    done = true
    let cls: AnyClass = type(of: delegate)
    let sel = #selector(UIApplicationDelegate.application(_:didRegisterForRemoteNotificationsWithDeviceToken:))
    let block: @convention(block) (AnyObject, UIApplication, Data) -> Void = { _, _, token in
      let hex = token.map { String(format: "%02x", $0) }.joined()
      NotificationCenter.default.post(name: Notification.Name("KiestoonAlertToken"), object: hex)
    }
    let imp = imp_implementationWithBlock(block)
    // v@:@@ : void, self, _cmd, application, token
    if !class_addMethod(cls, sel, imp, "v@:@@") {
      if let m = class_getInstanceMethod(cls, sel) { method_setImplementation(m, imp) }
    }
  }
}

// MARK: - plugin

class CallPlugin: Plugin, PKPushRegistryDelegate, CXProviderDelegate {
  private var registry: PKPushRegistry?
  private var provider: CXProvider?
  private let controller = CXCallController()
  private var voipToken = ""
  private var alertToken = ""
  /// The CallKit call of the last wake, with the wake id from the server.
  private var current: (uuid: UUID, wakeId: String)?
  /// An answer pressed before the page could take it.
  private var pending: [String: String]?
  private var giveUp: DispatchWorkItem?

  override func load(webview: WKWebView) {
    super.load(webview: webview)
    let cfg = CXProviderConfiguration()
    cfg.supportsVideo = false
    cfg.maximumCallGroups = 1
    cfg.maximumCallsPerCallGroup = 1
    cfg.supportedHandleTypes = [.phoneNumber, .generic]
    cfg.includesCallsInRecents = true
    let p = CXProvider(configuration: cfg)
    p.setDelegate(self, queue: nil)
    provider = p

    let r = PKPushRegistry(queue: .main)
    r.delegate = self
    r.desiredPushTypes = [.voIP]
    registry = r

    UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge]) { granted, _ in
      if granted {
        DispatchQueue.main.async { UIApplication.shared.registerForRemoteNotifications() }
      }
    }
    NotificationCenter.default.addObserver(
      self, selector: #selector(alertTokenChanged(_:)), name: Notification.Name("KiestoonAlertToken"), object: nil)
    AlertTokenHook.install()
    AVAudioSession.sharedInstance().requestRecordPermission { _ in }
  }

  /// The token of visible notifications, from the app delegate (see AlertTokenHook).
  @objc func alertTokenChanged(_ n: Notification) {
    guard let t = n.object as? String, t != alertToken else { return }
    alertToken = t
    trigger("token", data: ["token": voipToken, "alertToken": t])
  }

  // MARK: commands

  @objc public func registration(_ invoke: Invoke) {
    #if DEBUG
      let sandbox = true
    #else
      let sandbox = false
    #endif
    invoke.resolve([
      "platform": "apns", "token": voipToken, "alertToken": alertToken, "sandbox": sandbox,
      "appId": Store.appId, "name": UIDevice.current.name, "pushReady": !voipToken.isEmpty,
    ])
  }

  @objc public func setSecret(_ invoke: Invoke) throws {
    let a = try invoke.parseArgs(SecretArgs.self)
    Keychain.set("secret", a.secret)
    invoke.resolve()
  }

  @objc public func callUpdate(_ invoke: Invoke) throws {
    let a = try invoke.parseArgs(CallUpdateArgs.self)
    switch a.state {
    case "ringing":
      // The call of the wake has arrived over SIP: keep the CallKit call.
      giveUp?.cancel()
    case "active":
      giveUp?.cancel()
      if let c = current {
        provider?.reportOutgoingCall(with: c.uuid, connectedAt: Date())
      }
    case "ended":
      giveUp?.cancel()
      if let c = current {
        provider?.reportCall(with: c.uuid, endedAt: Date(), reason: .remoteEnded)
        current = nil
      }
    default:
      break
    }
    invoke.resolve()
  }

  @objc public func takePending(_ invoke: Invoke) {
    let p = pending ?? [:]
    pending = nil
    invoke.resolve(p)
  }

  // MARK: PushKit

  func pushRegistry(_ registry: PKPushRegistry, didUpdate credentials: PKPushCredentials, for type: PKPushType) {
    voipToken = credentials.token.map { String(format: "%02x", $0) }.joined()
    trigger("token", data: ["token": voipToken, "alertToken": alertToken])
  }

  func pushRegistry(_ registry: PKPushRegistry, didInvalidatePushTokenFor type: PKPushType) {
    voipToken = ""
  }

  func pushRegistry(
    _ registry: PKPushRegistry, didReceiveIncomingPushWith payload: PKPushPayload, for type: PKPushType,
    completion: @escaping () -> Void
  ) {
    let wakeId = payload.dictionaryPayload["id"] as? String ?? ""
    let data = Store.open(payload.dictionaryPayload["e"] as? String) ?? [:]
    let from = data["from"] as? String ?? ""
    let name = data["fromName"] as? String ?? ""
    let uuid = UUID()
    let update = CXCallUpdate()
    update.remoteHandle = CXHandle(type: from.isEmpty ? .generic : .phoneNumber, value: from.isEmpty ? "Kiestoon" : from)
    update.localizedCallerName = name.isEmpty ? (from.isEmpty ? nil : from) : name
    update.hasVideo = false
    update.supportsDTMF = true
    update.supportsHolding = true
    update.supportsGrouping = false
    update.supportsUngrouping = false
    // Report at once (iOS stops the app otherwise), also when a call is already shown.
    provider?.reportNewIncomingCall(with: uuid, update: update) { [weak self] err in
      if err == nil {
        self?.current = (uuid, wakeId)
        self?.scheduleGiveUp(uuid)
      }
      completion()
    }
    let json = (try? JSONSerialization.data(withJSONObject: data)).flatMap { String(data: $0, encoding: .utf8) } ?? ""
    trigger("push", data: ["kind": "call", "id": wakeId, "data": json])
  }

  /// When no call arrives (the caller hung up before the app connected), end the call screen.
  private func scheduleGiveUp(_ uuid: UUID) {
    giveUp?.cancel()
    let w = DispatchWorkItem { [weak self] in
      guard let self = self, self.current?.uuid == uuid else { return }
      self.provider?.reportCall(with: uuid, endedAt: Date(), reason: .unanswered)
      self.current = nil
    }
    giveUp = w
    DispatchQueue.main.asyncAfter(deadline: .now() + 25, execute: w)
  }

  // MARK: CallKit

  func providerDidReset(_ provider: CXProvider) {
    current = nil
  }

  func provider(_ provider: CXProvider, perform action: CXAnswerCallAction) {
    let id = current?.wakeId ?? ""
    pending = ["action": "answer", "id": id]
    trigger("callAction", data: ["action": "answer", "id": id])
    action.fulfill()
  }

  func provider(_ provider: CXProvider, perform action: CXEndCallAction) {
    let id = current?.wakeId ?? ""
    giveUp?.cancel()
    current = nil
    pending = ["action": "decline", "id": id]
    trigger("callAction", data: ["action": "end", "id": id])
    action.fulfill()
  }

  func provider(_ provider: CXProvider, perform action: CXSetMutedCallAction) {
    trigger("callAction", data: ["action": action.isMuted ? "mute" : "unmute", "id": current?.wakeId ?? ""])
    action.fulfill()
  }

  func provider(_ provider: CXProvider, perform action: CXSetHeldCallAction) {
    trigger("callAction", data: ["action": action.isOnHold ? "hold" : "unhold", "id": current?.wakeId ?? ""])
    action.fulfill()
  }
}

@_cdecl("init_plugin_kiestoon_call")
func initPlugin() -> Plugin {
  return CallPlugin()
}
