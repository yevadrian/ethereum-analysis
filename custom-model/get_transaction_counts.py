import re
import pandas as pd
from selenium import webdriver
from selenium.webdriver.common.by import By
from selenium.webdriver.chrome.service import Service
from selenium.webdriver.support.ui import WebDriverWait
from selenium.webdriver.support import expected_conditions as EC

# Selenium setup
service = Service("/usr/bin/chromedriver")
options = webdriver.ChromeOptions()
options.add_argument("--user-data-dir=/home/yevadrian/snap/chromium/common/chromium")
options.add_argument("--profile-directory=Default")
driver = webdriver.Chrome(service=service, options=options)

def out_transactions(address):
    try:
        driver.get(f"https://etherscan.io/advanced-filter?tadd={address}")
        wait = WebDriverWait(driver, 15)
        element = wait.until(EC.presence_of_element_located((By.CSS_SELECTOR, "p.text-muted.mb-0")))
        counts = int(re.search(r'\d+', element.text.replace(",", "")).group())
        print(f"Outgoing transaction counts for {address}: {counts}")
        return counts
    except Exception as exc:
        print(f"Failed to fetch outgoing transactions for {address}: {exc}")
        return None

def in_transactions(address):
    try:
        driver.get(f"https://etherscan.io/advanced-filter?fadd={address}")
        wait = WebDriverWait(driver, 15)
        element = wait.until(EC.presence_of_element_located((By.CSS_SELECTOR, "p.text-muted.mb-0")))
        counts = int(re.search(r'\d+', element.text.replace(",", "")).group())
        print(f"Incoming transaction counts for {address}: {counts}")
        return counts
    except Exception as exc:
        print(f"Failed to fetch incoming transactions for {address}: {exc}")
        return None

# Load addresses and labels
data = pd.read_csv('labeled_addresses.csv')  # Ensure the file has 'address' and 'label' columns
addresses = data['address'].tolist()
labels = data['label'].tolist()

try:
    # Resume from checkpoint if available
    with open('checkpoint.txt', 'r') as checkpoint_file:
        last_processed = checkpoint_file.read().strip()
        print(f"Resuming from last processed address: {last_processed}")
        start_index = addresses.index(last_processed) + 1
except FileNotFoundError:
    print("No checkpoint found, starting from the beginning.")
    start_index = 0

with open('labeled_addresses_transaction_counts.csv', 'a') as transaction_counts:
    if transaction_counts.tell() == 0:
        # Write header only if file is empty
        transaction_counts.write('address,label,outTxn,inTxn\n')

    # Process addresses
    for index, address in enumerate(addresses[start_index:], start=start_index):
        try:
            outTxn = out_transactions(address)
            inTxn = in_transactions(address)
            label = labels[index]
            transaction_counts.write(f'{address},{label},{outTxn},{inTxn}\n')
            transaction_counts.flush()  # Ensure data is saved immediately
            with open('checkpoint.txt', 'w') as checkpoint_file:
                checkpoint_file.write(address)

        except Exception as e:
            print(f"Error processing address {address}: {e}")
            break
