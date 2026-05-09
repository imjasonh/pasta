#include <iostream>
#include <vector>

using namespace std;                // want "pollutes the global namespace"

using std::cout;                    // OK: targeted using
using std::vector;                  // OK: targeted using

int main() {
    cout << "hello" << endl;
    vector<int> v;
    return 0;
}
