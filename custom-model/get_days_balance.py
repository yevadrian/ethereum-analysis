import requests
from bs4 import BeautifulSoup
import re
import time
import csv
import pandas as pd

# Load the CSV file
df = pd.read_csv('labeled_addresses.csv')

# Base Etherscan URL
base_url = "https://etherscan.io/address/"

# Output CSV file
output_file = 'labeled_addresses_days_balance.csv'

# Open the output CSV file in write mode
with open(output_file, mode='w', newline='', encoding='utf-8') as file:
    # Initialize CSV writer
    writer = csv.writer(file)
    
    # Write the header
    writer.writerow(['address', 'eth_balance', 'latest_days_ago', 'first_days_ago'])
    
    # Iterate through the addresses in the DataFrame
    for address in df['address']:
        url = f"{base_url}{address}"
        try:
            # Fetch the page content
            response = requests.get(url, headers={"User-Agent": "Mozilla/5.0"})
            
            if response.status_code == 200:
                soup = BeautifulSoup(response.text, 'html.parser')
                
                # ETH Balance Extraction
                balance_div = soup.find('div', class_='card-body')
                if balance_div:
                    balance_text = balance_div.text
                    eth_balance_match = re.search(r'(\d+\.\d+)', balance_text)
                    eth_balance = eth_balance_match.group() if eth_balance_match else "N/A"
                else:
                    eth_balance = "N/A"
                
                # Latest and First Transactions: Days Ago
                transactions_sent_div = soup.find('h4', text=re.compile(r'Transactions Sent'))
                if transactions_sent_div:
                    days_ago_spans = transactions_sent_div.find_next('div').find_all('span', text=re.compile(r'\d+ days ago'))
                    
                    # Extract the "days ago" values for latest and first
                    latest_days = re.search(r'\d+', days_ago_spans[0].text).group() if len(days_ago_spans) > 0 else "N/A"
                    first_days = re.search(r'\d+', days_ago_spans[1].text).group() if len(days_ago_spans) > 1 else "N/A"
                else:
                    latest_days = "N/A"
                    first_days = "N/A"
            else:
                print(f"Failed to fetch data for {address}, status code: {response.status_code}")
                eth_balance = "N/A"
                latest_days = "N/A"
                first_days = "N/A"
        except Exception as e:
            print(f"Error processing {address}: {e}")
            eth_balance = "N/A"
            latest_days = "N/A"
            first_days = "N/A"
        
        # Write to CSV and print to terminal
        row = [address, eth_balance, latest_days, first_days]
        writer.writerow(row)
        print(f"Address: {address}, ETH Balance: {eth_balance}, Latest Days Ago: {latest_days}, First Days Ago: {first_days}")
        
        # Add a delay to avoid rate-limiting
        time.sleep(1)

print(f"Data successfully fetched and saved to {output_file}!")