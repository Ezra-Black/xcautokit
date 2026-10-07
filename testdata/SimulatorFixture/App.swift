import UIKit

@main
final class AppDelegate: UIResponder, UIApplicationDelegate {
    var window: UIWindow?
    func application(_ application: UIApplication, didFinishLaunchingWithOptions options: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
        window = UIWindow(frame: UIScreen.main.bounds)
        window?.rootViewController = FixtureController()
        window?.makeKeyAndVisible()
        return true
    }
}

final class FixtureController: UIViewController {
    private var count = 0
    private let counter = UILabel()
    private let name = UITextField()
    private let result = UILabel()
    private let holdResult = UILabel()
    private let permissionResult = UILabel()

    override func viewDidLoad() {
        super.viewDidLoad()
        view.backgroundColor = .systemBackground
        let stack = UIStackView()
        stack.axis = .vertical
        stack.spacing = 12
        stack.translatesAutoresizingMaskIntoConstraints = false
        view.addSubview(stack)
        NSLayoutConstraint.activate([
            stack.leadingAnchor.constraint(equalTo: view.safeAreaLayoutGuide.leadingAnchor, constant: 24),
            stack.trailingAnchor.constraint(equalTo: view.safeAreaLayoutGuide.trailingAnchor, constant: -24),
            stack.topAnchor.constraint(equalTo: view.safeAreaLayoutGuide.topAnchor, constant: 12)
        ])
        let heading = UILabel()
        heading.text = "XCAutokit runtime fixture"
        heading.font = .preferredFont(forTextStyle: .headline)
        stack.addArrangedSubview(heading)
        counter.accessibilityIdentifier = "counter.value"
        updateCount()
        stack.addArrangedSubview(counter)
        stack.addArrangedSubview(makeButton("Increment", id: "counter.increment") { [weak self] in self?.increment() })
        stack.addArrangedSubview(makeButton("Reset", id: "counter.reset") { [weak self] in
            guard let self else { return }
            self.view.endEditing(true)
            self.count = 0
            self.updateCount()
            self.name.text = ""
            self.result.text = "Waiting"
            self.result.accessibilityIdentifier = "result-waiting"
            self.holdResult.text = "Not held"
            self.permissionResult.text = "No decision"
        })
        let duplicates = UIStackView()
        duplicates.distribution = .fillEqually
        for id in ["duplicate.first", "duplicate.second"] {
            duplicates.addArrangedSubview(makeButton("Duplicate", id: id) { [weak self] in self?.increment() })
        }
        stack.addArrangedSubview(duplicates)
        let disabled = makeButton("Disabled", id: "disabled") { [weak self] in self?.increment() }
        disabled.isEnabled = false
        stack.addArrangedSubview(disabled)
        name.placeholder = "Name"
        name.borderStyle = .roundedRect
        name.autocorrectionType = .no
        name.autocapitalizationType = .none
        name.smartDashesType = .no
        name.smartQuotesType = .no
        name.spellCheckingType = .no
        name.accessibilityIdentifier = "name-field"
        stack.addArrangedSubview(name)
        stack.addArrangedSubview(makeButton("Submit", id: "submit") { [weak self] in
            guard let self else { return }
            self.view.endEditing(true)
            let saved = self.name.text ?? ""
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.35) { [weak self] in
                self?.result.text = "Saved: \(saved)"
                self?.result.accessibilityIdentifier = "result-ready"
            }
        })
        result.text = "Waiting"
        result.accessibilityIdentifier = "result-waiting"
        stack.addArrangedSubview(result)
        stack.addArrangedSubview(makeButton("Show test alert", id: "permission-test") { [weak self] in
            let alert = UIAlertController(title: "Fixture would like to access your test data", message: "This is a fixture alert. No system permission is requested.", preferredStyle: .alert)
            alert.addAction(UIAlertAction(title: "Don't Allow", style: .cancel) { _ in self?.permissionResult.text = "Declined" })
            alert.addAction(UIAlertAction(title: "Allow", style: .default) { _ in self?.permissionResult.text = "Accepted" })
            self?.present(alert, animated: true)
        })
        permissionResult.text = "No decision"
        permissionResult.accessibilityIdentifier = "permission-result"
        stack.addArrangedSubview(permissionResult)
        let hold = makeButton("Hold for half a second", id: "hold-target") {}
        let recognizer = UILongPressGestureRecognizer(target: self, action: #selector(held(_:)))
        recognizer.minimumPressDuration = 0.5
        hold.addGestureRecognizer(recognizer)
        stack.addArrangedSubview(hold)
        holdResult.text = "Not held"
        holdResult.accessibilityIdentifier = "hold-result"
        stack.addArrangedSubview(holdResult)
    }

    private func makeButton(_ title: String, id: String, action: @escaping () -> Void) -> UIButton {
        let button = UIButton(type: .system)
        button.setTitle(title, for: .normal)
        button.accessibilityIdentifier = id
        button.addAction(UIAction { _ in action() }, for: .touchUpInside)
        button.heightAnchor.constraint(equalToConstant: 38).isActive = true
        return button
    }

    private func increment() { count += 1; updateCount() }
    private func updateCount() { counter.text = "Count: \(count)" }
    @objc private func held(_ gesture: UILongPressGestureRecognizer) {
        if gesture.state == .began { holdResult.text = "Held" }
    }
}
